package policy

import (
	"fmt"
	"sort"
	"time"
)

// Trigger is a change that invalidates prior assurance.
//
// The point of naming them is that each one has a victim: an eval result, a threat model, an
// approval — all of which describe the system as it was before the change. A trigger fired
// without revalidation means the project is relying on evidence about a system that no longer
// exists.
type Trigger struct {
	Name    string `json:"trigger"`
	AgentID string `json:"agent"`
	From    string `json:"from"`
	To      string `json:"to"`
	Why     string `json:"why"`
}

func (t Trigger) String() string {
	return fmt.Sprintf("%-24s %s\n    %s → %s\n    %s", t.Name, t.AgentID, val(t.From), val(t.To), t.Why)
}

func val(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

// DetectTriggers compares two versions of one agent's manifest.
//
// base may be nil, which is how a newly added agent is handled: everything about it is new, but
// nothing was invalidated, so no trigger fires.
func DetectTriggers(agentID string, base, head map[string]any) []Trigger {
	if base == nil || head == nil {
		return nil
	}

	var out []Trigger
	add := func(name, from, to, why string) {
		out = append(out, Trigger{Name: name, AgentID: agentID, From: from, To: to, Why: why})
	}

	bm, _ := base["model"].(map[string]any)
	hm, _ := head["model"].(map[string]any)
	if str(bm["id"]) != str(hm["id"]) {
		add("model_changed", str(bm["id"]), str(hm["id"]),
			"Every eval result and threat model taken before this describes a different system.")
	}
	if str(bm["provider"]) != str(hm["provider"]) {
		add("provider_changed", str(bm["provider"]), str(hm["provider"]),
			"Different provider, different behaviour, different data handling and different failure modes.")
	}

	bp, _ := base["prompts"].(map[string]any)
	hp, _ := head["prompts"].(map[string]any)
	bs, _ := bp["system"].(map[string]any)
	hs, _ := hp["system"].(map[string]any)
	if str(bs["sha256"]) != str(hs["sha256"]) {
		add("system_prompt_changed", shortHash(str(bs["sha256"])), shortHash(str(hs["sha256"])),
			"The prompt is the behaviour. Results measured against the old one no longer apply.")
	}

	bc, _ := base["context"].(map[string]any)
	hc, _ := head["context"].(map[string]any)
	br, _ := bc["rag"].(map[string]any)
	hr, _ := hc["rag"].(map[string]any)
	if str(br["corpus_version"]) != str(hr["corpus_version"]) {
		add("rag_corpus_changed", str(br["corpus_version"]), str(hr["corpus_version"]),
			"Groundedness and citation accuracy were measured against the previous corpus.")
	}

	ba, _ := base["autonomy"].(map[string]any)
	ha, _ := head["autonomy"].(map[string]any)
	if autonomyRank(str(ha["level"])) > autonomyRank(str(ba["level"])) {
		add("autonomy_raised", str(ba["level"]), str(ha["level"]),
			"The agent may now go further unattended than when it was last reviewed.")
	}

	baseTools := toolRefs(base)
	headTools := toolRefs(head)
	for ref := range headTools {
		if _, had := baseTools[ref]; !had {
			add("tool_added", "", ref,
				"A new capability widens the blast radius the threat model was written against.")
		}
	}

	baseMCP := mcpServers(base)
	headMCP := mcpServers(head)
	for s := range headMCP {
		if !baseMCP[s] {
			add("mcp_server_added", "", s,
				"A new server contributes tool descriptions the model treats as authoritative.")
		}
	}

	// A guardrail that disappears is the change most worth catching, because it usually
	// arrives as a small edit that makes something else pass.
	for _, point := range []string{"input", "output", "action"} {
		b := guardrailCount(base, point)
		h := guardrailCount(head, point)
		if h < b {
			add("guardrail_disabled", fmt.Sprintf("%s: %d", point, b), fmt.Sprintf("%s: %d", point, h),
				"A check was removed. Whatever it was catching is now reaching production.")
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func toolRefs(agent map[string]any) map[string]bool {
	out := map[string]bool{}
	if list, ok := agent["tools"].([]any); ok {
		for _, t := range list {
			if tm, ok := t.(map[string]any); ok {
				out[str(tm["ref"])] = true
			}
		}
	}
	return out
}

func mcpServers(agent map[string]any) map[string]bool {
	out := map[string]bool{}
	if m, ok := agent["mcp"].(map[string]any); ok {
		if list, ok := m["servers_used"].([]any); ok {
			for _, s := range list {
				out[str(s)] = true
			}
		}
	}
	return out
}

func guardrailCount(agent map[string]any, point string) int {
	g, ok := agent["guardrails"].(map[string]any)
	if !ok {
		return 0
	}
	l, _ := g[point].([]any)
	return len(l)
}

func shortHash(s string) string {
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}

// RevalidationDue reports whether a fired trigger has been answered.
//
// The test is simple and deliberately strict: last_validated_at must not predate the change. A
// project that changed the model on Tuesday and last validated on Monday is relying on evidence
// about a system that no longer exists.
func RevalidationDue(agent map[string]any, triggers []Trigger, changedAt time.Time) (bool, string) {
	if len(triggers) == 0 {
		return false, ""
	}

	lc, _ := agent["lifecycle"].(map[string]any)
	declared := map[string]bool{}
	if list, ok := lc["revalidate_on"].([]any); ok {
		for _, t := range list {
			declared[str(t)] = true
		}
	}

	var relevant []string
	for _, t := range triggers {
		if declared[t.Name] {
			relevant = append(relevant, t.Name)
		}
	}
	if len(relevant) == 0 {
		return false, ""
	}

	last := str(lc["last_validated_at"])
	if last == "" {
		return true, fmt.Sprintf("%v fired and last_validated_at is unset", relevant)
	}
	d, err := time.Parse("2006-01-02", last)
	if err != nil {
		return true, "last_validated_at is not a YYYY-MM-DD date"
	}
	if d.Before(changedAt.Truncate(24 * time.Hour)) {
		return true, fmt.Sprintf("%v fired, but the agent was last validated on %s", relevant, last)
	}
	return false, ""
}
