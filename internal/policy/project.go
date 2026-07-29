package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Agent is one agent's manifest plus everything the expression context needs alongside it.
type Agent struct {
	ID       string
	Dir      string
	Manifest map[string]any
	Ctx      map[string]any
}

// LoadAgents reads every agent under <root>/agentarch/project/agents, resolving its tool specs,
// MCP allowlist and eval result into the evaluation context.
//
// Resolution happens here rather than inside the expression language on purpose: a language
// that can read files is a language that can be pointed at files it should not read.
func LoadAgents(root string, now time.Time) ([]Agent, error) {
	base := filepath.Join(root, "agentarch", "project", "agents")
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no agents found in %s — run `agentarch init` and `agentarch new agent <id>`", base)
	}
	if err != nil {
		return nil, err
	}

	var out []Agent
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(base, e.Name())
		manifestPath := filepath.Join(dir, "agent.yaml")
		doc, err := readYAML(manifestPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", manifestPath, err)
		}
		agentNode, _ := doc["agent"].(map[string]any)
		if agentNode == nil {
			return nil, fmt.Errorf("%s: no `agent` key", manifestPath)
		}

		ctx := map[string]any{
			"agent": agentNode,
			"now":   now.Format("2006-01-02"),
		}

		// Tools: each entry is the tool document merged with the manifest's per-tool
		// settings, so a control can reason about both in one expression.
		var tools []any
		if list, ok := agentNode["tools"].([]any); ok {
			for _, t := range list {
				tm, _ := t.(map[string]any)
				ref, _ := tm["ref"].(string)
				if ref == "" {
					continue
				}
				td, err := readYAML(filepath.Join(dir, ref))
				if err != nil {
					// A broken ref is reported by validate (AA-REF-002). Here it
					// becomes an entry with no tool body, so tool controls fail
					// rather than passing over a file that could not be read.
					tools = append(tools, map[string]any{"ref": ref, "tool": nil})
					continue
				}
				merged := map[string]any{"ref": ref, "tool": td["tool"]}
				if v, ok := tm["approval"]; ok {
					merged["approval"] = v
				}
				if v, ok := tm["rate_limit"]; ok {
					merged["rate_limit"] = v
				}
				tools = append(tools, merged)
			}
		}
		ctx["tools"] = tools

		// MCP allowlist.
		if mcp, ok := agentNode["mcp"].(map[string]any); ok {
			if ref, _ := mcp["allowlist_ref"].(string); ref != "" {
				if ad, err := readYAML(filepath.Join(dir, ref)); err == nil {
					ctx["mcp"] = ad
					if servers, ok := ad["servers"].([]any); ok {
						ctx["mcp_servers"] = servers
					}
				}
			}
		}

		// Eval result, when one is referenced and readable. Absent is meaningfully
		// different from stale, and the freshness control distinguishes them.
		if ev, ok := agentNode["evaluation"].(map[string]any); ok {
			if ref, _ := ev["last_result_ref"].(string); ref != "" {
				if ed, err := readYAML(filepath.Join(dir, ref)); err == nil {
					ctx["evals"] = ed
				}
			}
		}
		if _, ok := ctx["evals"]; !ok {
			ctx["evals"] = map[string]any{}
		}

		id, _ := agentNode["id"].(string)
		out = append(out, Agent{ID: id, Dir: dir, Manifest: doc, Ctx: ctx})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// readYAML loads YAML through JSON so the evaluation context contains only the generic types
// the expression language defines. Without the round trip, yaml.v3's map[any]any leaks in and
// field lookups silently return nothing.
func readYAML(path string) (map[string]any, error) {
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
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("document is not a mapping")
	}
	return m, nil
}

// PacksFor determines which packs apply to an agent: the profile's set, plus anything the
// manifest itself asks for. Union rather than override — an agent may add obligations to its
// project's profile, never remove them.
func PacksFor(profile string, agentNode map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		// Manifests in the wild carry versioned ids like "core.agent.v1"; normalise.
		id = strings.TrimSuffix(id, ".v1")
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, p := range Profiles[profile] {
		add(p)
	}
	if pol, ok := agentNode["policy"].(map[string]any); ok {
		if list, ok := pol["packs"].([]any); ok {
			for _, p := range list {
				if s, ok := p.(string); ok {
					add(s)
				}
			}
		}
	}
	return out
}

// Summary aggregates results for exit-code and reporting purposes.
type Summary struct {
	Total     int
	Passed    int
	Skipped   int
	Waived    int
	Baselined int
	Blockers  []Result
	Majors    []Result
	Minors    []Result
	Warns     []Result
	Errors    []Result
}

// Summarize partitions results. A waived failure counts as waived, not as passed: the debt
// stays visible in the score even while the gate lets it through.
func Summarize(results []Result) Summary {
	var s Summary
	for _, r := range results {
		s.Total++
		switch {
		case r.Error != "":
			s.Errors = append(s.Errors, r)
		case r.Skipped:
			s.Skipped++
			s.Passed++
		case r.Passed:
			s.Passed++
		case r.Waived:
			s.Waived++
		case r.Baselined:
			// Not a pass. The gate lets it through; the score still counts it against the
			// project, because a ratchet that hides its debt is an amnesty.
			s.Baselined++
		default:
			switch r.Severity {
			case SevBlocker:
				s.Blockers = append(s.Blockers, r)
			case SevMajor:
				s.Majors = append(s.Majors, r)
			case SevMinor:
				s.Minors = append(s.Minors, r)
			case SevWarn:
				s.Warns = append(s.Warns, r)
			}
		}
	}
	return s
}
