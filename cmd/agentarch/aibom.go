package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/mcp"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

// cmdAIBOM generates an AI bill of materials from the manifests.
//
// An SBOM lists packages. It does not list the model, the prompt version, the retrieval corpus
// or the MCP servers — which is most of what determines how an agent behaves. Generating this
// from the manifests is the point: an AI-BOM maintained by hand is a document about a system
// that used to exist.
func cmdAIBOM(args []string) int {
	fs_ := flag.NewFlagSet("aibom", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	out := fs_.String("out", "", "write here instead of stdout")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	now := time.Now().UTC()
	agents, err := policy.LoadAgents(*root, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aibom:", err)
		return exitUsage
	}

	type component struct {
		Type       string         `json:"type"`
		Name       string         `json:"name"`
		Version    string         `json:"version,omitempty"`
		Supplier   string         `json:"supplier,omitempty"`
		Hash       string         `json:"sha256,omitempty"`
		Pinned     *bool          `json:"pinned,omitempty"`
		Properties map[string]any `json:"properties,omitempty"`
	}
	type agentEntry struct {
		ID         string      `json:"id"`
		Stage      string      `json:"stage"`
		SystemType string      `json:"system_type"`
		Owner      string      `json:"owner"`
		Components []component `json:"components"`
	}

	doc := map[string]any{
		"bom_format":   "agentarch-aibom",
		"spec_version": "1.0",
		"generated_at": now.Format(time.RFC3339),
		"generated_by": "agentarch " + version,
		"note": "Generated from the manifests. Do not edit by hand — a hand-maintained AI-BOM " +
			"is a document about a system that used to exist.",
	}

	var entries []agentEntry
	for _, a := range agents {
		ag, _ := a.Ctx["agent"].(map[string]any)
		owner, _ := ag["owner"].(map[string]any)
		e := agentEntry{
			ID:         a.ID,
			Stage:      strOf(ag["stage"]),
			SystemType: strOf(ag["system_type"]),
			Owner:      strOf(owner["accountable"]),
		}

		if m, ok := ag["model"].(map[string]any); ok {
			pinned, _ := m["pinned"].(bool)
			e.Components = append(e.Components, component{
				Type: "model", Name: strOf(m["id"]), Supplier: strOf(m["provider"]),
				Pinned: &pinned,
			})
		}

		if p, ok := ag["prompts"].(map[string]any); ok {
			if s, ok := p["system"].(map[string]any); ok {
				e.Components = append(e.Components, component{
					Type: "prompt", Name: strOf(s["path"]),
					Version: strOf(s["version"]), Hash: strOf(s["sha256"]),
				})
			}
		}

		if c, ok := ag["context"].(map[string]any); ok {
			if r, ok := c["rag"].(map[string]any); ok {
				if enabled, _ := r["enabled"].(bool); enabled {
					e.Components = append(e.Components, component{
						Type: "corpus", Name: strOf(r["corpus_id"]),
						Version: strOf(r["corpus_version"]),
						Properties: map[string]any{
							"citation_required": r["citation_required"],
							"untrusted":         true,
						},
					})
				}
			}
		}

		if tools, ok := a.Ctx["tools"].([]any); ok {
			for _, t := range tools {
				tm, _ := t.(map[string]any)
				td, _ := tm["tool"].(map[string]any)
				if td == nil {
					continue
				}
				e.Components = append(e.Components, component{
					Type: "tool", Name: strOf(td["id"]), Supplier: strOf(td["owner"]),
					Properties: map[string]any{
						"effect":   td["effect"],
						"approval": tm["approval"],
					},
				})
			}
		}

		// MCP servers are supply-chain components that can also write the prompt, so the
		// digest of each reviewed description belongs in the bill of materials.
		if al, _, err := mcp.Load(*root); err == nil {
			for _, s := range al.Servers {
				e.Components = append(e.Components, component{
					Type: "mcp_server", Name: s.Name,
					Version: s.Pin.Version, Supplier: s.Pin.Package,
					Hash: s.Pin.Integrity,
					Properties: map[string]any{
						"trust":                   s.Trust,
						"tools_allow":             s.ToolsAllow,
						"reviewed_at":             s.ReviewedAt,
						"reviewer":                s.Reviewer,
						"tool_description_sha256": s.ToolDescriptionSHA,
					},
				})
			}
		}

		entries = append(entries, e)
	}
	doc["agents"] = entries

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "aibom:", err)
		return exitUsage
	}
	b = append(b, '\n')

	if *out == "" {
		os.Stdout.Write(b)
		return exitOK
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "aibom:", err)
		return exitUsage
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "aibom:", err)
		return exitUsage
	}
	total := 0
	for _, e := range entries {
		total += len(e.Components)
	}
	fmt.Printf("wrote %s — %d agent(s), %d component(s)\n", *out, len(entries), total)
	return exitOK
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}
