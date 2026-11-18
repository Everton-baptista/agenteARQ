package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// The MCP server's declared filesystem scope is the selected project. Reference fields
// must not turn a read-only validation call into an arbitrary host-file reader.
func confined(root, base, ref string) error {
	if ref == "" {
		return nil
	}
	if filepath.IsAbs(ref) {
		return fmt.Errorf("artifact reference must be project-relative: %s", ref)
	}
	target := filepath.Clean(filepath.Join(base, ref))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact reference escapes the project: %s", ref)
	}
	path := root
	for _, piece := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, piece)
		st, e := os.Lstat(path)
		if os.IsNotExist(e) {
			break
		}
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in policy artifact reference: %s", ref)
		}
	}
	return nil
}
func confineArtifacts(root string) error {
	abs, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	root = abs
	base := filepath.Join(root, "agentarch", "project")
	if e = confined(root, root, "agentarch/project"); e != nil {
		return e
	}
	if e = confined(root, root, "agentarch/agentarch.yaml"); e != nil {
		return e
	}
	if e = confined(root, root, "agentarch/std"); e != nil {
		return e
	}
	std := filepath.Join(root, "agentarch", "std")
	if _, err := os.Stat(std); err == nil {
		if err = filepath.WalkDir(std, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink not allowed in installed standard: %s", path)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in policy artifacts: %s", path)
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "agent.yaml" && d.Name() != "plan.yaml" {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var doc map[string]any
		if e = yaml.Unmarshal(b, &doc); e != nil {
			return nil
		} // schema validation reports malformed artifacts
		base := filepath.Dir(path)
		if a, ok := doc["agent"].(map[string]any); ok {
			refs := []string{}
			get := func(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
			str := func(m map[string]any, key string) string { v, _ := m[key].(string); return v }
			refs = append(refs, str(get(get(a, "prompts"), "system"), "path"), str(get(a, "mcp"), "allowlist_ref"), str(get(a, "evaluation"), "plan_ref"), str(get(a, "evaluation"), "last_result_ref"), str(get(a, "policy"), "waivers_ref"))
			if tools, ok := a["tools"].([]any); ok {
				for _, t := range tools {
					if m, ok := t.(map[string]any); ok {
						refs = append(refs, str(m, "ref"))
					}
				}
			}
			if handoff, ok := get(a, "handoff")["hands_off_to"].([]any); ok {
				for _, h := range handoff {
					if m, ok := h.(map[string]any); ok {
						refs = append(refs, str(m, "payload_schema"))
					}
				}
			}
			for _, ref := range refs {
				if e = confined(root, base, ref); e != nil {
					return e
				}
			}
			planRef := str(get(a, "evaluation"), "plan_ref")
			if planRef != "" {
				if e = confinePlan(root, filepath.Join(base, planRef)); e != nil {
					return e
				}
			}
		}
		if p, ok := doc["plan"].(map[string]any); ok {
			if sets, ok := p["datasets"].([]any); ok {
				for _, entry := range sets {
					if m, ok := entry.(map[string]any); ok {
						ref, _ := m["path"].(string)
						if e = confined(root, base, ref); e != nil {
							return e
						}
					}
				}
			}
		}
		return nil
	})
}

func confinePlan(root, path string) error {
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var doc struct {
		Plan struct {
			Datasets []struct {
				Path string `yaml:"path"`
			} `yaml:"datasets"`
		} `yaml:"plan"`
	}
	if yaml.Unmarshal(b, &doc) != nil {
		return nil
	}
	for _, d := range doc.Plan.Datasets {
		if e = confined(root, filepath.Dir(path), d.Path); e != nil {
			return e
		}
	}
	return nil
}
