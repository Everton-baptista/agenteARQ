// Package blueprint provides complete, working starting points.
//
// The rest of agentarch tells you when an agent is wrong. This tells you where to start, which
// is the half people actually need first: a standard that can only reject is a standard nobody
// reaches for on day one.
//
// A blueprint is a whole project — manifest, prompt, tools, evals, threat model, CI — that
// already passes the gate, plus code that runs. You edit it rather than assembling it.
package blueprint

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Meta is blueprint.yaml: what this starting point is for, and what it actually ships.
type Meta struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	// Need is the sentence someone recognises themselves in. The catalogue is browsed by
	// need, not by id, because nobody arrives knowing the id.
	Need         string   `yaml:"need" json:"need"`
	Description  string   `yaml:"description" json:"description"`
	SystemType   string   `yaml:"system_type" json:"system_type"`
	Demonstrates []string `yaml:"demonstrates" json:"demonstrates"`
	// Frameworks lists what this blueprint genuinely ships runnable code for. Claiming more
	// than that would be the same dishonesty the standard exists to prevent.
	Frameworks  []string `yaml:"frameworks" json:"frameworks"`
	Provider    string   `yaml:"provider" json:"provider"`
	Conformance string   `yaml:"conformance" json:"conformance"`
}

// Blueprint is a loaded catalogue entry.
type Blueprint struct {
	Meta Meta
	Root string // path within the content tree, e.g. "blueprints/rag-support"
}

// Load reads every blueprint from a content tree.
func Load(fsys fs.FS) ([]Blueprint, error) {
	entries, err := fs.ReadDir(fsys, "blueprints")
	if err != nil {
		return nil, nil // a content tree with no blueprints is not an error
	}

	var out []Blueprint
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		root := path.Join("blueprints", e.Name())
		raw, err := fs.ReadFile(fsys, path.Join(root, "blueprint.yaml"))
		if err != nil {
			continue
		}
		var m Meta
		if err := yaml.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", root, err)
		}
		if m.ID == "" {
			return nil, fmt.Errorf("%s: blueprint has no id", root)
		}
		out = append(out, Blueprint{Meta: m, Root: root})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

// Find looks a blueprint up by id.
func Find(bps []Blueprint, id string) (Blueprint, bool) {
	for _, b := range bps {
		if b.Meta.ID == id {
			return b, true
		}
	}
	return Blueprint{}, false
}

// HasFramework reports whether this blueprint ships runnable code for a framework.
func (b Blueprint) HasFramework(name string) bool {
	for _, f := range b.Meta.Frameworks {
		if f == name {
			return true
		}
	}
	return false
}

// Conflict is a file the blueprint would write over.
type Conflict struct{ Path string }

// Plan is what an install would do, computed before anything is written.
type Plan struct {
	Files     []string
	Conflicts []Conflict
	Framework string
}

// Prepare computes the install without touching disk, so the caller can show it and ask.
//
// Writing first and reporting afterwards is how a scaffolding tool loses someone's work. The
// plan is shown, then confirmed, then applied.
func Prepare(fsys fs.FS, b Blueprint, dest, framework string) (*Plan, error) {
	if framework == "" {
		framework = b.Meta.Frameworks[0]
	}
	if !b.HasFramework(framework) {
		return nil, fmt.Errorf(
			"blueprint %s does not ship code for %q.\nIt ships: %s\n"+
				"Adapting it is documented in agentarch/std/adapters/%s.md",
			b.Meta.ID, framework, strings.Join(b.Meta.Frameworks, ", "), framework)
	}

	p := &Plan{Framework: framework}

	err := fs.WalkDir(fsys, b.Root, func(src string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(b.Root, src)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)

		if rel == "blueprint.yaml" {
			return nil // catalogue metadata, not part of the project
		}
		// app/<framework>/… collapses to app/… so only the chosen one is written.
		if strings.HasPrefix(rel, "app/") {
			parts := strings.SplitN(rel, "/", 3)
			if len(parts) < 3 || parts[1] != framework {
				return nil
			}
			rel = "app/" + parts[2]
		}

		p.Files = append(p.Files, rel)
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err == nil {
			p.Conflicts = append(p.Conflicts, Conflict{rel})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(p.Files)
	return p, nil
}

// Apply writes the blueprint. Callers pass a plan they have already shown to the user.
func Apply(fsys fs.FS, b Blueprint, dest string, p *Plan) error {
	for _, rel := range p.Files {
		src := path.Join(b.Root, rel)
		if strings.HasPrefix(rel, "app/") {
			src = path.Join(b.Root, "app", p.Framework, strings.TrimPrefix(rel, "app/"))
		}
		body, err := fs.ReadFile(fsys, src)
		if err != nil {
			return fmt.Errorf("%s: %w", src, err)
		}
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(out, body, mode); err != nil {
			return err
		}
	}
	return nil
}

// ByNeed groups the catalogue for display. Someone arriving does not know the ids; they know
// what they are trying to build.
func ByNeed(bps []Blueprint) []Blueprint {
	out := append([]Blueprint(nil), bps...)
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.Need < out[j].Meta.Need })
	return out
}
