package policy_test

import (
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func manifest(mut func(m map[string]any)) map[string]any {
	m := map[string]any{
		"id":       "triage",
		"model":    map[string]any{"id": "claude-x-1", "provider": "anthropic", "pinned": true},
		"prompts":  map[string]any{"system": map[string]any{"sha256": "aaaa1111"}},
		"context":  map[string]any{"rag": map[string]any{"corpus_version": "2026-07-01"}},
		"autonomy": map[string]any{"level": "L1_act_with_approval"},
		"tools":    []any{map[string]any{"ref": "../../tools/search.tool.yaml"}},
		"mcp":      map[string]any{"servers_used": []any{"docs"}},
		"guardrails": map[string]any{
			"input":  []any{map[string]any{"control": "control.ai.genai.prompt_injection"}},
			"output": []any{map[string]any{"control": "control.ai.privacy.pii_leakage"}},
			"action": []any{map[string]any{"control": "control.ai.tool.least_privilege"}},
		},
		"lifecycle": map[string]any{
			"revalidate_on": []any{
				"model_changed", "system_prompt_changed", "rag_corpus_changed",
				"provider_changed", "tool_added", "autonomy_raised",
				"guardrail_disabled", "mcp_server_added",
			},
			"last_validated_at": "2026-07-20",
		},
	}
	if mut != nil {
		mut(m)
	}
	return m
}

func names(ts []policy.Trigger) map[string]bool {
	m := map[string]bool{}
	for _, t := range ts {
		m[t.Name] = true
	}
	return m
}

func TestNoChangeFiresNothing(t *testing.T) {
	if ts := policy.DetectTriggers("triage", manifest(nil), manifest(nil)); len(ts) != 0 {
		t.Fatalf("expected no triggers, got %v", ts)
	}
}

func TestEachTriggerFires(t *testing.T) {
	cases := []struct {
		want string
		mut  func(m map[string]any)
	}{
		{"model_changed", func(m map[string]any) {
			m["model"].(map[string]any)["id"] = "claude-x-2"
		}},
		{"provider_changed", func(m map[string]any) {
			m["model"].(map[string]any)["provider"] = "bedrock"
		}},
		{"system_prompt_changed", func(m map[string]any) {
			m["prompts"].(map[string]any)["system"].(map[string]any)["sha256"] = "bbbb2222"
		}},
		{"rag_corpus_changed", func(m map[string]any) {
			m["context"].(map[string]any)["rag"].(map[string]any)["corpus_version"] = "2026-08-01"
		}},
		{"autonomy_raised", func(m map[string]any) {
			m["autonomy"].(map[string]any)["level"] = "L3_act_irreversible_bounded"
		}},
		{"tool_added", func(m map[string]any) {
			m["tools"] = append(m["tools"].([]any),
				map[string]any{"ref": "../../tools/refund.tool.yaml"})
		}},
		{"mcp_server_added", func(m map[string]any) {
			m["mcp"].(map[string]any)["servers_used"] = []any{"docs", "billing"}
		}},
		{"guardrail_disabled", func(m map[string]any) {
			m["guardrails"].(map[string]any)["action"] = []any{}
		}},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := names(policy.DetectTriggers("triage", manifest(nil), manifest(c.mut)))
			if !got[c.want] {
				t.Fatalf("expected %s to fire, got %v", c.want, got)
			}
		})
	}
}

// Lowering autonomy narrows what the agent may do alone, so nothing prior is invalidated.
func TestLoweringAutonomyDoesNotFire(t *testing.T) {
	base := manifest(func(m map[string]any) {
		m["autonomy"].(map[string]any)["level"] = "L3_act_irreversible_bounded"
	})
	head := manifest(nil) // back to L1
	if names(policy.DetectTriggers("triage", base, head))["autonomy_raised"] {
		t.Fatal("lowering autonomy must not fire autonomy_raised")
	}
}

// Removing a tool shrinks the blast radius; the threat model still covers the remainder.
func TestRemovingAToolDoesNotFire(t *testing.T) {
	base := manifest(nil)
	head := manifest(func(m map[string]any) { m["tools"] = []any{} })
	if names(policy.DetectTriggers("triage", base, head))["tool_added"] {
		t.Fatal("removing a tool must not fire tool_added")
	}
}

// A new agent has no prior assurance to invalidate.
func TestNewAgentFiresNothing(t *testing.T) {
	if ts := policy.DetectTriggers("triage", nil, manifest(nil)); len(ts) != 0 {
		t.Fatalf("a newly added agent must not fire triggers, got %v", ts)
	}
}

func TestRevalidationDueWhenValidationPredatesTheChange(t *testing.T) {
	changed := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	head := manifest(func(m map[string]any) {
		m["model"].(map[string]any)["id"] = "claude-x-2"
	})
	ts := policy.DetectTriggers("triage", manifest(nil), head)

	due, reason := policy.RevalidationDue(head, ts, changed)
	if !due {
		t.Fatal("a model change on the 28th with validation on the 20th must be due")
	}
	if reason == "" {
		t.Error("the reason should name the trigger and the stale date")
	}
}

func TestRevalidationNotDueWhenAlreadyValidated(t *testing.T) {
	changed := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	head := manifest(func(m map[string]any) {
		m["model"].(map[string]any)["id"] = "claude-x-2"
		m["lifecycle"].(map[string]any)["last_validated_at"] = "2026-07-28"
	})
	ts := policy.DetectTriggers("triage", manifest(nil), head)
	if due, _ := policy.RevalidationDue(head, ts, changed); due {
		t.Fatal("validation on the day of the change satisfies the trigger")
	}
}

// A trigger the agent never declared does not create an obligation. Declaring the list is the
// project's decision about which changes it treats as invalidating.
func TestUndeclaredTriggerDoesNotCreateAnObligation(t *testing.T) {
	changed := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	head := manifest(func(m map[string]any) {
		m["model"].(map[string]any)["id"] = "claude-x-2"
		m["lifecycle"].(map[string]any)["revalidate_on"] = []any{"rag_corpus_changed"}
	})
	ts := policy.DetectTriggers("triage", manifest(nil), head)
	if due, _ := policy.RevalidationDue(head, ts, changed); due {
		t.Fatal("model_changed is not in revalidate_on, so it must not be due")
	}
}

func TestMissingValidationDateIsDue(t *testing.T) {
	changed := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	head := manifest(func(m map[string]any) {
		m["model"].(map[string]any)["id"] = "claude-x-2"
		delete(m["lifecycle"].(map[string]any), "last_validated_at")
	})
	ts := policy.DetectTriggers("triage", manifest(nil), head)
	if due, _ := policy.RevalidationDue(head, ts, changed); !due {
		t.Fatal("a trigger with no recorded validation must be due")
	}
}
