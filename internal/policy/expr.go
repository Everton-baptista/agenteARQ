// Package policy evaluates controls against a project's artifacts.
//
// The expression language implemented here is deliberately total and closed: it terminates, it
// allocates boundedly, and it can reach nothing but the evaluation context. Packs are data that
// travels — through a registry, from third parties — and a governance tool that executes
// third-party code in order to verify governance hands an execution primitive to anyone who can
// get a pack adopted.
//
// The normative definition is spec/normative/04-expression-language.md.
package policy

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Limits from the spec. Exceeding one is an error, never a failed check — a control that
// silently reports false because it ran out of budget is worse than one that is absent.
const (
	MaxExprLen   = 4096
	MaxDepth     = 64
	MaxMulti     = 10000
	MaxEvalSteps = 100000
)

// Multi is the result of a `[]` traversal: every element, carried forward so that the operators
// after it apply element-wise.
type Multi []any

// ---------------------------------------------------------------- lexer

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNumber
	tString
	tOp
	tLParen
	tRParen
	tLBracket
	tRBracket
	tComma
	tDot
)

type token struct {
	kind tokKind
	s    string
	n    float64
	pos  int
}

var multiCharOps = []string{"==", "!=", "<=", ">="}

func lex(src string) ([]token, error) {
	var out []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, token{kind: tLParen, pos: i})
			i++
		case c == ')':
			out = append(out, token{kind: tRParen, pos: i})
			i++
		case c == '[':
			out = append(out, token{kind: tLBracket, pos: i})
			i++
		case c == ']':
			out = append(out, token{kind: tRBracket, pos: i})
			i++
		case c == ',':
			out = append(out, token{kind: tComma, pos: i})
			i++
		case c == '.':
			out = append(out, token{kind: tDot, pos: i})
			i++
		case c == '"' || c == '\'':
			s, n, err := lexString(src, i)
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: tString, s: s, pos: i})
			i = n
		case c >= '0' && c <= '9':
			j := i
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				return nil, fmt.Errorf("bad number at %d: %q", i, src[i:j])
			}
			out = append(out, token{kind: tNumber, n: f, pos: i})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentChar(src[j]) {
				j++
			}
			out = append(out, token{kind: tIdent, s: src[i:j], pos: i})
			i = j
		default:
			matched := false
			for _, op := range multiCharOps {
				if strings.HasPrefix(src[i:], op) {
					out = append(out, token{kind: tOp, s: op, pos: i})
					i += len(op)
					matched = true
					break
				}
			}
			if !matched {
				if c == '<' || c == '>' {
					out = append(out, token{kind: tOp, s: string(c), pos: i})
					i++
				} else {
					return nil, fmt.Errorf("unexpected character %q at %d", string(c), i)
				}
			}
		}
	}
	out = append(out, token{kind: tEOF, pos: len(src)})
	return out, nil
}

func lexString(src string, start int) (string, int, error) {
	quote := src[start]
	var b strings.Builder
	i := start + 1
	for i < len(src) {
		if src[i] == '\\' && i+1 < len(src) {
			b.WriteByte(src[i+1])
			i += 2
			continue
		}
		if src[i] == quote {
			return b.String(), i + 1, nil
		}
		b.WriteByte(src[i])
		i++
	}
	return "", 0, fmt.Errorf("unterminated string at %d", start)
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
func isIdentChar(c byte) bool { return isIdentStart(c) || c >= '0' && c <= '9' }

// ---------------------------------------------------------------- AST

type node interface{}

type nLiteral struct{ v any }
type nList struct{ items []node }
type nPath struct {
	root  string
	steps []pathStep
}
type pathStep struct {
	field string
	multi bool
	index int
	isIdx bool
}
type nCall struct {
	fn   string
	args []node
}
type nBinary struct {
	op   string
	l, r node
}
type nUnary struct {
	op string
	x  node
}

// ---------------------------------------------------------------- parser

type parser struct {
	toks  []token
	i     int
	depth int
}

// Parse compiles an expression. A malformed expression is an error, never a false result.
func Parse(src string) (node, error) {
	if len(src) > MaxExprLen {
		return nil, fmt.Errorf("expression is %d bytes, over the %d byte limit", len(src), MaxExprLen)
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.cur().kind != tEOF {
		return nil, fmt.Errorf("unexpected trailing input at %d", p.cur().pos)
	}
	return n, nil
}

func (p *parser) cur() token  { return p.toks[p.i] }
func (p *parser) next() token { t := p.toks[p.i]; p.i++; return t }

func (p *parser) enter() error {
	p.depth++
	if p.depth > MaxDepth {
		return fmt.Errorf("expression nested deeper than %d", MaxDepth)
	}
	return nil
}
func (p *parser) leave() { p.depth-- }

func (p *parser) isKeyword(s string) bool {
	t := p.cur()
	return t.kind == tIdent && t.s == s
}

func (p *parser) parseOr() (node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	l, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") {
		p.next()
		r, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l = &nBinary{op: "or", l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseAnd() (node, error) {
	l, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		p.next()
		r, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l = &nBinary{op: "and", l: l, r: r}
	}
	return l, nil
}

func (p *parser) parseUnary() (node, error) {
	if p.isKeyword("not") {
		// "not in" is a comparison operator, not a unary negation.
		if p.toks[p.i+1].kind == tIdent && p.toks[p.i+1].s == "in" {
			return p.parseComparison()
		}
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &nUnary{op: "not", x: x}, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (node, error) {
	l, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	t := p.cur()
	switch {
	case t.kind == tOp:
		p.next()
		r, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &nBinary{op: t.s, l: l, r: r}, nil
	case t.kind == tIdent && t.s == "in":
		p.next()
		r, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &nBinary{op: "in", l: l, r: r}, nil
	case t.kind == tIdent && t.s == "not" && p.toks[p.i+1].kind == tIdent && p.toks[p.i+1].s == "in":
		p.next()
		p.next()
		r, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &nUnary{op: "not", x: &nBinary{op: "in", l: l, r: r}}, nil
	}
	return l, nil
}

func (p *parser) parsePrimary() (node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	t := p.cur()
	switch t.kind {
	case tNumber:
		p.next()
		return &nLiteral{v: t.n}, nil
	case tString:
		p.next()
		return &nLiteral{v: t.s}, nil
	case tLParen:
		p.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != tRParen {
			return nil, fmt.Errorf("missing ) at %d", p.cur().pos)
		}
		p.next()
		return n, nil
	case tLBracket:
		p.next()
		var items []node
		for p.cur().kind != tRBracket {
			it, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			items = append(items, it)
			if p.cur().kind == tComma {
				p.next()
			} else {
				break
			}
		}
		if p.cur().kind != tRBracket {
			return nil, fmt.Errorf("missing ] at %d", p.cur().pos)
		}
		p.next()
		return &nList{items: items}, nil
	case tIdent:
		switch t.s {
		case "true":
			p.next()
			return &nLiteral{v: true}, nil
		case "false":
			p.next()
			return &nLiteral{v: false}, nil
		case "null":
			p.next()
			return &nLiteral{v: nil}, nil
		}
		if p.toks[p.i+1].kind == tLParen {
			return p.parseCall()
		}
		return p.parsePath()
	}
	return nil, fmt.Errorf("unexpected token at %d", t.pos)
}

func (p *parser) parseCall() (node, error) {
	name := p.next().s
	if _, ok := functions[name]; !ok {
		return nil, fmt.Errorf("unknown function %q — the function set is closed", name)
	}
	p.next() // (
	var args []node
	for p.cur().kind != tRParen {
		a, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.cur().kind == tComma {
			p.next()
		} else {
			break
		}
	}
	if p.cur().kind != tRParen {
		return nil, fmt.Errorf("missing ) in call to %s", name)
	}
	p.next()
	return &nCall{fn: name, args: args}, nil
}

// reserved words can never be a path root or a field name. Without this, a bare `and` parses as
// a lookup of a field called "and", evaluates to null, and reports a clean `false` — a
// malformed control that silently passes, which is the failure mode this language exists to
// avoid.
var reserved = map[string]bool{"and": true, "or": true, "not": true, "in": true}

func (p *parser) parsePath() (node, error) {
	root := p.next()
	if reserved[root.s] {
		return nil, fmt.Errorf("%q is a reserved word and cannot be used as a name (at %d)", root.s, root.pos)
	}
	pn := &nPath{root: root.s}
	for {
		switch p.cur().kind {
		case tDot:
			p.next()
			if p.cur().kind != tIdent {
				return nil, fmt.Errorf("expected field name after . at %d", p.cur().pos)
			}
			pn.steps = append(pn.steps, pathStep{field: p.next().s})
		case tLBracket:
			p.next()
			if p.cur().kind == tRBracket {
				p.next()
				pn.steps = append(pn.steps, pathStep{multi: true})
				continue
			}
			if p.cur().kind != tNumber {
				return nil, fmt.Errorf("only [] or a numeric index is allowed at %d", p.cur().pos)
			}
			idx := int(p.next().n)
			if p.cur().kind != tRBracket {
				return nil, fmt.Errorf("missing ] at %d", p.cur().pos)
			}
			p.next()
			pn.steps = append(pn.steps, pathStep{index: idx, isIdx: true})
		default:
			return pn, nil
		}
	}
}

// ---------------------------------------------------------------- evaluator

type evaluator struct {
	ctx   map[string]any
	steps int
	now   time.Time
}

// Eval evaluates a parsed expression against a context and reduces the result to a boolean.
func Eval(n node, ctx map[string]any, now time.Time) (bool, error) {
	e := &evaluator{ctx: ctx, now: now}
	v, err := e.eval(n)
	if err != nil {
		return false, err
	}
	// A multi that reaches the top level was never reduced. Requiring the author to say
	// whether they meant all() or any() removes a whole class of silently-wrong control:
	// "some tool denies egress" and "every tool denies egress" are very different claims.
	if m, ok := v.(Multi); ok {
		return false, fmt.Errorf(
			"expression yields %d values rather than a single boolean — wrap it in all() or any()", len(m))
	}
	return truthy(v), nil
}

// EvalString parses and evaluates in one step.
func EvalString(src string, ctx map[string]any, now time.Time) (bool, error) {
	n, err := Parse(src)
	if err != nil {
		return false, err
	}
	return Eval(n, ctx, now)
}

func (e *evaluator) tick() error {
	e.steps++
	if e.steps > MaxEvalSteps {
		return fmt.Errorf("evaluation exceeded %d steps", MaxEvalSteps)
	}
	return nil
}

func (e *evaluator) eval(n node) (any, error) {
	if err := e.tick(); err != nil {
		return nil, err
	}
	switch t := n.(type) {
	case *nLiteral:
		return t.v, nil
	case *nList:
		out := make([]any, 0, len(t.items))
		for _, it := range t.items {
			v, err := e.eval(it)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case *nPath:
		return e.evalPath(t)
	case *nCall:
		return e.evalCall(t)
	case *nUnary:
		v, err := e.eval(t.x)
		if err != nil {
			return nil, err
		}
		if m, ok := v.(Multi); ok {
			out := make(Multi, len(m))
			for i, x := range m {
				out[i] = !truthy(x)
			}
			return out, nil
		}
		return !truthy(v), nil
	case *nBinary:
		return e.evalBinary(t)
	}
	return nil, fmt.Errorf("unsupported node %T", n)
}

func (e *evaluator) evalPath(p *nPath) (any, error) {
	// A missing root is null rather than an error, and the steps still run: `tools[]` on a
	// context with no tools must yield an empty multi, so that a control about tools holds
	// vacuously instead of failing an agent that has none.
	var vals any = e.ctx[p.root]
	for _, st := range p.steps {
		if err := e.tick(); err != nil {
			return nil, err
		}
		switch {
		case st.multi:
			list, ok := toList(vals)
			if !ok {
				// A [] over null or a non-list is an empty multi, not an error: a
				// control about tools must hold vacuously for an agent with none.
				vals = Multi{}
				continue
			}
			if len(list) > MaxMulti {
				return nil, fmt.Errorf("multi cardinality %d exceeds %d", len(list), MaxMulti)
			}
			vals = Multi(list)
		case st.isIdx:
			list, ok := toList(vals)
			if !ok || st.index < 0 || st.index >= len(list) {
				vals = nil
				continue
			}
			vals = list[st.index]
		default:
			vals = fieldOf(vals, st.field)
		}
	}
	return vals, nil
}

// fieldOf projects a field, mapping element-wise across a multi.
func fieldOf(v any, field string) any {
	if m, ok := v.(Multi); ok {
		out := make(Multi, len(m))
		for i, x := range m {
			out[i] = fieldOf(x, field)
		}
		return out
	}
	switch t := v.(type) {
	case map[string]any:
		return t[field]
	case map[any]any:
		return t[field]
	}
	return nil
}

func (e *evaluator) evalBinary(b *nBinary) (any, error) {
	// Short-circuit boolean operators, but only when neither side is a multi.
	if b.op == "and" || b.op == "or" {
		l, err := e.eval(b.l)
		if err != nil {
			return nil, err
		}
		if _, isMulti := l.(Multi); !isMulti {
			if b.op == "and" && !truthy(l) {
				return false, nil
			}
			if b.op == "or" && truthy(l) {
				return true, nil
			}
			r, err := e.eval(b.r)
			if err != nil {
				return nil, err
			}
			if _, isMulti := r.(Multi); !isMulti {
				return truthy(r), nil
			}
			return zip(l, r, func(x, y any) any { return boolOp(b.op, x, y) })
		}
		r, err := e.eval(b.r)
		if err != nil {
			return nil, err
		}
		return zip(l, r, func(x, y any) any { return boolOp(b.op, x, y) })
	}

	l, err := e.eval(b.l)
	if err != nil {
		return nil, err
	}
	r, err := e.eval(b.r)
	if err != nil {
		return nil, err
	}
	return zip(l, r, func(x, y any) any { return compare(b.op, x, y) })
}

// zip applies f element-wise when either operand is a multi.
func zip(l, r any, f func(x, y any) any) (any, error) {
	lm, lIsMulti := l.(Multi)
	rm, rIsMulti := r.(Multi)
	switch {
	case !lIsMulti && !rIsMulti:
		return f(l, r), nil
	case lIsMulti && !rIsMulti:
		out := make(Multi, len(lm))
		for i, x := range lm {
			out[i] = f(x, r)
		}
		return out, nil
	case !lIsMulti && rIsMulti:
		out := make(Multi, len(rm))
		for i, y := range rm {
			out[i] = f(l, y)
		}
		return out, nil
	default:
		if len(lm) != len(rm) {
			return nil, fmt.Errorf("cannot combine multis of length %d and %d", len(lm), len(rm))
		}
		out := make(Multi, len(lm))
		for i := range lm {
			out[i] = f(lm[i], rm[i])
		}
		return out, nil
	}
}

func boolOp(op string, x, y any) any {
	if op == "and" {
		return truthy(x) && truthy(y)
	}
	return truthy(x) || truthy(y)
}

func compare(op string, l, r any) any {
	switch op {
	case "==":
		return valuesEqual(l, r)
	case "!=":
		return !valuesEqual(l, r)
	case "in":
		return contains(r, l)
	}

	lf, lok := toNumber(l)
	rf, rok := toNumber(r)
	if !lok || !rok {
		// Dates compare lexically, which is correct for YYYY-MM-DD.
		ls, lIsStr := l.(string)
		rs, rIsStr := r.(string)
		if lIsStr && rIsStr && isDateLike(ls) && isDateLike(rs) {
			return compareOrdered(op, strings.Compare(ls, rs))
		}
		return nil // an incomparable comparison is null, and null is falsy
	}
	switch {
	case lf < rf:
		return compareOrdered(op, -1)
	case lf > rf:
		return compareOrdered(op, 1)
	default:
		return compareOrdered(op, 0)
	}
}

func compareOrdered(op string, c int) bool {
	switch op {
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	}
	return false
}

func contains(haystack, needle any) bool {
	switch h := haystack.(type) {
	case []any:
		for _, x := range h {
			if valuesEqual(x, needle) {
				return true
			}
		}
	case Multi:
		for _, x := range h {
			if valuesEqual(x, needle) {
				return true
			}
		}
	case string:
		if s, ok := needle.(string); ok {
			return strings.Contains(h, s)
		}
	case map[string]any:
		if s, ok := needle.(string); ok {
			_, found := h[s]
			return found
		}
	}
	return false
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if af, ok := toNumber(a); ok {
		if bf, ok := toNumber(b); ok {
			return math.Abs(af-bf) < 1e-9
		}
		return false
	}
	al, aIsList := toList(a)
	bl, bIsList := toList(b)
	if aIsList && bIsList {
		if len(al) != len(bl) {
			return false
		}
		for i := range al {
			if !valuesEqual(al[i], bl[i]) {
				return false
			}
		}
		return true
	}
	if aIsList != bIsList {
		return false
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case Multi:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	if f, ok := toNumber(v); ok {
		return f != 0
	}
	return true
}

func toNumber(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case uint64:
		return float64(t), true
	}
	return 0, false
}

func toList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case Multi:
		return []any(t), true
	}
	return nil, false
}

func isDateLike(s string) bool {
	return len(s) >= 10 && s[4] == '-' && s[7] == '-'
}

// ---------------------------------------------------------------- functions

var functions = map[string]int{
	"all": 1, "any": 1, "len": 1, "exists": 1,
	"matches": 2, "age_days": 1, "date": 1, "lower": 1, "upper": 1,
}

// reCache keeps compiled patterns. Go's regexp is RE2 and runs in linear time, which satisfies
// the spec's requirement that a pack cannot supply a denial-of-service through a pattern.
var reCache = map[string]*regexp.Regexp{}

func (e *evaluator) evalCall(c *nCall) (any, error) {
	want := functions[c.fn]
	if len(c.args) != want {
		return nil, fmt.Errorf("%s takes %d argument(s), got %d", c.fn, want, len(c.args))
	}
	args := make([]any, len(c.args))
	for i, a := range c.args {
		v, err := e.eval(a)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}

	switch c.fn {
	case "all":
		m, ok := args[0].(Multi)
		if !ok {
			return truthy(args[0]), nil
		}
		for _, x := range m {
			if !truthy(x) {
				return false, nil
			}
		}
		return true, nil // vacuously true when empty

	case "any":
		m, ok := args[0].(Multi)
		if !ok {
			return truthy(args[0]), nil
		}
		for _, x := range m {
			if truthy(x) {
				return true, nil
			}
		}
		return false, nil

	case "len":
		switch t := args[0].(type) {
		case string:
			return float64(len(t)), nil
		case []any:
			return float64(len(t)), nil
		case Multi:
			return float64(len(t)), nil
		case map[string]any:
			return float64(len(t)), nil
		}
		return float64(0), nil

	case "exists":
		return truthy(args[0]), nil

	case "matches":
		s, ok := args[0].(string)
		if !ok {
			return false, nil
		}
		pat, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("matches expects a string pattern")
		}
		re, cached := reCache[pat]
		if !cached {
			var err error
			re, err = regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("bad regular expression %q: %w", pat, err)
			}
			reCache[pat] = re
		}
		return re.MatchString(s), nil

	case "age_days":
		d, ok := parseDate(args[0])
		if !ok {
			return nil, nil // missing or unparseable: null, so comparisons are false
		}
		return math.Floor(e.now.Sub(d).Hours() / 24), nil

	case "date":
		if _, ok := parseDate(args[0]); !ok {
			return nil, nil
		}
		s, _ := args[0].(string)
		return s, nil

	case "lower":
		s, _ := args[0].(string)
		return strings.ToLower(s), nil

	case "upper":
		s, _ := args[0].(string)
		return strings.ToUpper(s), nil
	}
	return nil, fmt.Errorf("unknown function %q", c.fn)
}

func parseDate(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
