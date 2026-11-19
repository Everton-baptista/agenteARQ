// Package validate checks artifacts for structure and internal consistency.
//
// It deliberately makes no policy judgement. "Is this well formed and self consistent" and "is
// this allowed here" are different questions with different answers per project, and merging
// them is how a standard becomes something teams route around. Policy lives in packs and is
// evaluated by the gate.
package validate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

// Finding is one problem. IDs are stable so they can be cited in review without ambiguity.
type Finding struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Pointer string `json:"pointer,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

func (f Finding) String() string {
	loc := f.File
	if f.Pointer != "" {
		loc += " " + f.Pointer
	}
	s := fmt.Sprintf("%s  %s\n    %s", f.ID, loc, f.Message)
	if f.Fix != "" {
		s += "\n    fix: " + f.Fix
	}
	return s
}

// Validator holds the compiled schemas.
type Validator struct {
	agent *jsonschema.Schema
	tool  *jsonschema.Schema
}

// New compiles the schemas out of the embedded spec tree.
func New(specFS fs.FS) (*Validator, error) {
	c := jsonschema.NewCompiler()

	load := func(name, id string) (*jsonschema.Schema, error) {
		raw, err := fs.ReadFile(specFS, "spec/schemas/"+name)
		if err != nil {
			return nil, err
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := c.AddResource(id, doc); err != nil {
			return nil, err
		}
		return c.Compile(id)
	}

	a, err := load("agent.manifest.schema.json", "agent.manifest.schema.json")
	if err != nil {
		return nil, err
	}
	t, err := load("tool.spec.schema.json", "tool.spec.schema.json")
	if err != nil {
		return nil, err
	}
	return &Validator{agent: a, tool: t}, nil
}

// yamlToJSON reads a YAML file into the generic form the schema validator expects. The JSON
// round trip is not decoration: yaml.v3 produces types the validator does not recognise, and
// silently skipping those keys would make validation pass on documents it never inspected.
func yamlToJSON(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var y any
	if err := yaml.Unmarshal(raw, &y); err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}
	b, err := json.Marshal(y)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
}

// printer is required because jsonschema's BasicOutput leaves its message printer nil, and
// OutputError.String dereferences it. Passing an explicit one is the supported path.
var printer = message.NewPrinter(language.English)

func schemaFindings(id, file string, err error) []Finding {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []Finding{{ID: id, File: file, Message: err.Error()}}
	}

	var out []Finding
	seen := map[string]bool{}

	// Only leaves carry a usable message. The interior nodes say "doesn't validate with
	// <subschema>", which tells a reader nothing they can act on; the leaf says "'approval'
	// is a required property", which tells them exactly what to add.
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		msg := e.ErrorKind.LocalizedString(printer)
		if msg == "" {
			return
		}
		ptr := "/" + strings.Join(e.InstanceLocation, "/")
		key := ptr + "\x00" + msg
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Finding{ID: id, File: file, Pointer: ptr, Message: msg})
	}
	walk(ve)

	if len(out) == 0 {
		out = append(out, Finding{ID: id, File: file, Message: ve.Error()})
	}
	return out
}

// Project validates every artifact under <root>/agentarch/project.
func (v *Validator) Project(root string) ([]Finding, error) {
	var findings []Finding
	base := filepath.Join(root, "agentarch", "project")
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil, fmt.Errorf("no agentarch/project directory in %s — run `agentarch init` first", root)
	}

	agentIDs := map[string]string{}
	toolIDs := map[string]string{}

	err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		switch {
		case name == "agent.yaml":
			findings = append(findings, v.agentFile(p, agentIDs)...)
		case strings.HasSuffix(name, ".tool.yaml"):
			findings = append(findings, v.toolFile(p, toolIDs)...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// AA-DEP-019, here rather than only in the CLI. It lived in the command for a while, which meant
	// `agentarch validate` reported a layer violation and the library did not — so the test that was
	// supposed to prove the rule is enforced could not see it. A check the CLI has and the library
	// lacks is a check nothing tests.
	findings = append(findings, LintLayers(root, ReadLayout(root))...)

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].ID < findings[j].ID
	})
	return findings, nil
}

// ReadLayout pulls layout.paths out of agentarch.yaml.
//
// Its own small parser rather than a YAML dependency, and in this package rather than the command, so
// that whoever calls Project gets the same answer the CLI does.
func ReadLayout(root string) Layout {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return Layout{}
	}

	var out Layout
	inLayout, inPaths := false, false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if indent == 0 && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			// A new top-level key ends the block. Without this, a `paths:` under some later key
			// would be read as if it were the layout's.
			inLayout = strings.HasPrefix(trimmed, "layout:")
			inPaths = false
			continue
		}
		if !inLayout || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "paths:") {
			inPaths = true
			continue
		}
		if !inPaths {
			continue
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		list := parseInlineList(value)
		switch strings.TrimSpace(key) {
		case "edge":
			out.Edge = list
		case "core":
			out.Core = list
		case "client":
			out.Client = list
		}
	}
	return out
}

// parseInlineList reads ["a", "b"] and a bare scalar alike.
func parseInlineList(v string) []string {
	v = strings.Trim(strings.TrimSpace(v), "[]")
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.Trim(strings.TrimSpace(part), `"'`); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (v *Validator) agentFile(p string, seen map[string]string) []Finding {
	doc, err := yamlToJSON(p)
	if err != nil {
		return []Finding{{ID: "AA-SCH-001", File: p, Message: err.Error()}}
	}
	var out []Finding
	if err := v.agent.Validate(doc); err != nil {
		out = append(out, schemaFindings("AA-SCH-001", p, err)...)
	}

	m, _ := doc.(map[string]any)
	ag, _ := m["agent"].(map[string]any)
	if ag == nil {
		return out
	}
	dir := filepath.Dir(p)

	// AA-DUP-006 — a duplicate id makes every later reference ambiguous.
	if id, ok := ag["id"].(string); ok {
		if prev, dup := seen[id]; dup {
			out = append(out, Finding{ID: "AA-DUP-006", File: p,
				Message: fmt.Sprintf("agent id %q is already used by %s", id, prev),
				Fix:     "ids must be unique across the project"})
		} else {
			seen[id] = p
		}
	}

	// AA-REF-004 — the recorded hash must match the prompt on disk. This is the check that
	// catches a prompt edited without a version bump, which is otherwise an invisible
	// behaviour change that invalidates every eval result taken before it.
	if prompts, ok := ag["prompts"].(map[string]any); ok {
		if sysp, ok := prompts["system"].(map[string]any); ok {
			rel, _ := sysp["path"].(string)
			want, _ := sysp["sha256"].(string)
			full := filepath.Join(dir, rel)
			raw, rerr := os.ReadFile(full)
			switch {
			case rerr != nil:
				out = append(out, Finding{ID: "AA-REF-004", File: p, Pointer: "/agent/prompts/system/path",
					Message: fmt.Sprintf("system prompt %q not found", rel)})
			default:
				sum := sha256.Sum256(raw)
				got := hex.EncodeToString(sum[:])
				if got != want {
					out = append(out, Finding{ID: "AA-REF-004", File: p, Pointer: "/agent/prompts/system/sha256",
						Message: fmt.Sprintf("prompt %s has changed: manifest records %s…, file hashes to %s…",
							rel, short(want), short(got)),
						Fix: "bump prompts.system.version and update sha256 — a prompt change is a behaviour change"})
				}
			}
		}
	}

	// AA-REF-002 — every referenced tool spec must exist.
	if tools, ok := ag["tools"].([]any); ok {
		for _, t := range tools {
			tm, _ := t.(map[string]any)
			ref, _ := tm["ref"].(string)
			if ref == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, ref)); err != nil {
				out = append(out, Finding{ID: "AA-REF-002", File: p, Pointer: "/agent/tools",
					Message: fmt.Sprintf("tool ref %q does not resolve", ref)})
			}
		}
	}
	return out
}

func (v *Validator) toolFile(p string, seen map[string]string) []Finding {
	doc, err := yamlToJSON(p)
	if err != nil {
		return []Finding{{ID: "AA-SCH-001", File: p, Message: err.Error()}}
	}
	var out []Finding
	if err := v.tool.Validate(doc); err != nil {
		out = append(out, schemaFindings("AA-SCH-001", p, err)...)
	}
	if m, ok := doc.(map[string]any); ok {
		if tl, ok := m["tool"].(map[string]any); ok {
			if id, ok := tl["id"].(string); ok {
				if prev, dup := seen[id]; dup {
					out = append(out, Finding{ID: "AA-DUP-006", File: p,
						Message: fmt.Sprintf("tool id %q is already used by %s", id, prev)})
				} else {
					seen[id] = p
				}
			}
		}
	}
	return out
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
