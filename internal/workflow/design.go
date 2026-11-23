package workflow

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Generated interfaces converge on the same look because the model samples the centre of its
// training data. A ban list only moves the centre. What holds is a declared direction (the
// brief) and a small, named set of tokens that every screen must use, checked on the file.
//
// The checks here are the deterministic part of the design standard: the tokens parse as W3C
// Design Tokens (DTCG 2025.10), aliases resolve without cycles, every typeface records its
// licence, and the colour pairs the product declares meet WCAG 2 contrast. Taste is not
// checked here; it is advice from a reviewer, never a gate.

const DesignBriefPath = "design/brief.md"
const DesignTokensPath = "design/tokens.json"

// designExtension is the namespace AgenteARQ reads inside DTCG `$extensions`.
const designExtension = "dev.agentarch"

var (
	briefSubject    = []string{"subject", "assunto", "tema", "sujeto", "product", "produto", "producto"}
	briefAudience   = []string{"audience", "users", "público", "publico", "usuários", "usuarios", "audiencia"}
	briefReferences = []string{"references", "referências", "referencias", "inspiration", "inspiração", "inspiración"}
	aliasRef        = regexp.MustCompile(`^\{([^{}]+)\}$`)
	hexColor        = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// contrastMinimum is WCAG 2.x: 4.5:1 for body text, 3:1 for large text and UI components.
var contrastMinimum = map[string]float64{"text": 4.5, "large": 3, "ui": 3}

type designToken struct {
	path       string
	typ        string
	value      any
	extensions map[string]any
}

// designGaps checks the design artifacts that exist. Absent files are advice, not gaps.
func (s *Service) designGaps() []string {
	var gaps []string
	if s.exists(DesignBriefPath) {
		gaps = append(gaps, s.briefGaps()...)
	}
	if s.exists(DesignTokensPath) {
		p, _ := safePath(s.Root, DesignTokensPath)
		b, e := os.ReadFile(p)
		if e != nil {
			return append(gaps, DesignTokensPath+": "+e.Error())
		}
		gaps = append(gaps, TokenGaps(b)...)
	}
	return gaps
}

func (s *Service) briefGaps() []string {
	p, _ := safePath(s.Root, DesignBriefPath)
	b, e := os.ReadFile(p)
	if e != nil {
		return []string{DesignBriefPath + ": " + e.Error()}
	}
	_, sections := markdownSections(string(b), "## ")
	var gaps []string
	for _, want := range []struct {
		name  string
		words []string
	}{{"Subject", briefSubject}, {"Audience", briefAudience}, {"References", briefReferences}} {
		body, ok := sectionContaining(sections, want.words)
		if !ok {
			gaps = append(gaps, fmt.Sprintf("%s needs a \"## %s\" section", DesignBriefPath, want.name))
		} else if !meaningful(body) {
			gaps = append(gaps, fmt.Sprintf("%s section %q must be filled in without TODO", DesignBriefPath, want.name))
		}
	}
	return gaps
}

// sectionContaining finds the first heading that contains one of the words, so "## Público-alvo"
// and "## Target audience" both count.
func sectionContaining(sections map[string]string, words []string) (string, bool) {
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, w := range words {
			if strings.Contains(k, w) {
				return sections[k], true
			}
		}
	}
	return "", false
}

// TokenGaps validates a DTCG tokens document. It is exported so the CLI and tests can check a
// file without a workflow.
func TokenGaps(raw []byte) []string {
	var root map[string]any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if e := d.Decode(&root); e != nil {
		return []string{DesignTokensPath + " is not a JSON object: " + e.Error()}
	}
	tokens := map[string]*designToken{}
	var gaps []string
	collectTokens(root, "", "", tokens, &gaps)
	if len(tokens) == 0 {
		gaps = append(gaps, DesignTokensPath+" defines no tokens")
	}
	paths := make([]string, 0, len(tokens))
	for p := range tokens {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	colors := 0
	for _, p := range paths {
		t := tokens[p]
		v, typ, e := resolveToken(tokens, p, map[string]bool{})
		if e != nil {
			gaps = append(gaps, e.Error())
			continue
		}
		if typ == "" {
			gaps = append(gaps, "token "+p+" has no $type (on the token, a parent group or the token it aliases)")
			continue
		}
		switch typ {
		case "color":
			colors++
			if _, e := parseColor(v); e != nil {
				gaps = append(gaps, "token "+p+": "+e.Error())
			}
		case "fontFamily":
			if _, isAlias := aliasTarget(t.value); !isAlias && extensionString(t.extensions, "license") == "" {
				gaps = append(gaps, "font token "+p+" must record its licence in $extensions[\""+designExtension+"\"].license")
			}
		}
	}
	gaps = append(gaps, contrastGaps(root, tokens, colors)...)
	return gaps
}

// collectTokens walks groups. A node with $value is a token; any other object is a group whose
// $type is inherited by its descendants.
func collectTokens(node map[string]any, prefix, inherited string, out map[string]*designToken, gaps *[]string) {
	if t, ok := node["$type"].(string); ok {
		inherited = t
	}
	keys := make([]string, 0, len(node))
	for k := range node {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.HasPrefix(k, "$") {
			continue
		}
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if strings.ContainsAny(k, "{}.") {
			*gaps = append(*gaps, "token or group name "+path+" must not contain {, } or .")
			continue
		}
		child, ok := node[k].(map[string]any)
		if !ok {
			*gaps = append(*gaps, path+" must be a token or group object")
			continue
		}
		if v, isToken := child["$value"]; isToken {
			t := &designToken{path: path, typ: inherited, value: v}
			if own, ok := child["$type"].(string); ok {
				t.typ = own
			}
			if ext, ok := child["$extensions"].(map[string]any); ok {
				if ours, ok := ext[designExtension].(map[string]any); ok {
					t.extensions = ours
				}
			}
			out[path] = t
			continue
		}
		collectTokens(child, path, inherited, out, gaps)
	}
}

func aliasTarget(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	m := aliasRef.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// resolveToken follows aliases to a concrete value. A token without its own $type takes the
// type of the token it aliases, as DTCG specifies.
func resolveToken(tokens map[string]*designToken, path string, visiting map[string]bool) (any, string, error) {
	t, ok := tokens[path]
	if !ok {
		return nil, "", fmt.Errorf("alias {%s} points to no token", path)
	}
	if visiting[path] {
		return nil, "", fmt.Errorf("token %s is part of an alias cycle", path)
	}
	target, isAlias := aliasTarget(t.value)
	if !isAlias {
		return t.value, t.typ, nil
	}
	visiting[path] = true
	defer delete(visiting, path)
	v, typ, e := resolveToken(tokens, target, visiting)
	if e != nil {
		return nil, "", e
	}
	if t.typ != "" {
		typ = t.typ
	}
	return v, typ, nil
}

func extensionString(ext map[string]any, key string) string {
	if ext == nil {
		return ""
	}
	s, _ := ext[key].(string)
	return strings.TrimSpace(s)
}

type rgba struct{ r, g, b, a float64 } // linear-light sRGB, alpha 0..1

// parseColor accepts a DTCG 2025.10 colour object (srgb, srgb-linear or oklch) or a hex string.
func parseColor(v any) (rgba, error) {
	switch c := v.(type) {
	case string:
		return parseHex(c)
	case map[string]any:
		space, _ := c["colorSpace"].(string)
		comps, ok := c["components"].([]any)
		if !ok || len(comps) != 3 {
			return rgba{}, fmt.Errorf("colour needs three components")
		}
		n := [3]float64{}
		for i, x := range comps {
			f, e := number(x)
			if e != nil {
				return rgba{}, fmt.Errorf("colour component %d: %v", i+1, e)
			}
			n[i] = f
		}
		alpha := 1.0
		if a, present := c["alpha"]; present {
			f, e := number(a)
			if e != nil || f < 0 || f > 1 {
				return rgba{}, fmt.Errorf("colour alpha must be between 0 and 1")
			}
			alpha = f
		}
		switch space {
		case "srgb":
			return rgba{linearize(n[0]), linearize(n[1]), linearize(n[2]), alpha}, nil
		case "srgb-linear":
			return rgba{clamp01(n[0]), clamp01(n[1]), clamp01(n[2]), alpha}, nil
		case "oklch":
			r, g, b := oklchToLinearSRGB(n[0], n[1], n[2])
			return rgba{r, g, b, alpha}, nil
		default:
			return rgba{}, fmt.Errorf("colour space %q is not checked yet; use srgb, srgb-linear or oklch", space)
		}
	}
	return rgba{}, fmt.Errorf("colour must be a DTCG colour object or a hex string")
}

// number reads a JSON number; the DTCG keyword "none" is a missing component, treated as 0.
func number(x any) (float64, error) {
	switch v := x.(type) {
	case json.Number:
		return v.Float64()
	case float64:
		return v, nil
	case string:
		if v == "none" {
			return 0, nil
		}
	}
	return 0, fmt.Errorf("not a number")
}

func parseHex(s string) (rgba, error) {
	if !hexColor.MatchString(s) {
		return rgba{}, fmt.Errorf("%q is not a hex colour", s)
	}
	h := s[1:]
	if len(h) <= 4 {
		var b strings.Builder
		for _, ch := range h {
			b.WriteRune(ch)
			b.WriteRune(ch)
		}
		h = b.String()
	}
	ch := func(i int) float64 { v, _ := strconv.ParseUint(h[i:i+2], 16, 8); return float64(v) / 255 }
	c := rgba{linearize(ch(0)), linearize(ch(2)), linearize(ch(4)), 1}
	if len(h) == 8 {
		c.a = ch(6)
	}
	return c, nil
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// linearize removes the sRGB transfer curve from a 0..1 channel.
func linearize(c float64) float64 {
	c = clamp01(c)
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// oklchToLinearSRGB converts OKLCH (L 0..1, C, H in degrees) to linear sRGB, clamping colours
// outside the sRGB gamut. Clamping is what a browser does with an out-of-gamut colour on an
// sRGB display, so the contrast is computed for what most people will see.
func oklchToLinearSRGB(l, c, h float64) (float64, float64, float64) {
	hr := h * math.Pi / 180
	a, b := c*math.Cos(hr), c*math.Sin(hr)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	L, M, S := l_*l_*l_, m_*m_*m_, s_*s_*s_
	r := 4.0767416621*L - 3.3077115913*M + 0.2309699292*S
	g := -1.2684380046*L + 2.6097574011*M - 0.3413193965*S
	bl := -0.0041960863*L - 0.7034186147*M + 1.7076147010*S
	return clamp01(r), clamp01(g), clamp01(bl)
}

func luminance(c rgba) float64 { return 0.2126*c.r + 0.7152*c.g + 0.0722*c.b }

// ContrastRatio is the WCAG 2 ratio of a foreground composited over an opaque background.
func ContrastRatio(fg, bg rgba) float64 {
	if fg.a < 1 {
		fg = rgba{fg.r*fg.a + bg.r*(1-fg.a), fg.g*fg.a + bg.g*(1-fg.a), fg.b*fg.a + bg.b*(1-fg.a), 1}
	}
	l1, l2 := luminance(fg), luminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// contrastGaps checks the pairs declared at the root under $extensions["dev.agentarch"].contrast.
// A palette with colours but no declared pairs is a gap: without pairs there is nothing to check,
// and "no pairs" would otherwise read as "contrast passed".
func contrastGaps(root map[string]any, tokens map[string]*designToken, colors int) []string {
	var pairs []any
	if ext, ok := root["$extensions"].(map[string]any); ok {
		if ours, ok := ext[designExtension].(map[string]any); ok {
			pairs, _ = ours["contrast"].([]any)
		}
	}
	if colors > 0 && len(pairs) == 0 {
		return []string{DesignTokensPath + " must declare the foreground/background pairs it uses in $extensions[\"" + designExtension + "\"].contrast"}
	}
	var gaps []string
	for i, raw := range pairs {
		pair, ok := raw.(map[string]any)
		if !ok {
			gaps = append(gaps, fmt.Sprintf("contrast pair %d must be an object with fg, bg and use", i+1))
			continue
		}
		use, _ := pair["use"].(string)
		min, known := contrastMinimum[use]
		if !known {
			gaps = append(gaps, fmt.Sprintf("contrast pair %d use must be text, large or ui", i+1))
			continue
		}
		fg, e1 := pairColor(tokens, pair["fg"])
		bg, e2 := pairColor(tokens, pair["bg"])
		if e1 != nil || e2 != nil {
			for _, e := range []error{e1, e2} {
				if e != nil {
					gaps = append(gaps, fmt.Sprintf("contrast pair %d: %v", i+1, e))
				}
			}
			continue
		}
		if bg.a < 1 {
			gaps = append(gaps, fmt.Sprintf("contrast pair %d: the background must be opaque to be checked", i+1))
			continue
		}
		if ratio := ContrastRatio(fg, bg); ratio < min {
			gaps = append(gaps, fmt.Sprintf("contrast %s on %s is %.2f:1; %s needs at least %.1f:1 (WCAG 2)", pair["fg"], pair["bg"], ratio, use, min))
		}
	}
	return gaps
}

func pairColor(tokens map[string]*designToken, ref any) (rgba, error) {
	s, _ := ref.(string)
	path := s
	if target, ok := aliasTarget(s); ok {
		path = target
	}
	if path == "" {
		return rgba{}, fmt.Errorf("fg and bg must name colour tokens")
	}
	v, typ, e := resolveToken(tokens, path, map[string]bool{})
	if e != nil {
		return rgba{}, e
	}
	if typ != "color" {
		return rgba{}, fmt.Errorf("%s is not a colour token", path)
	}
	return parseColor(v)
}
