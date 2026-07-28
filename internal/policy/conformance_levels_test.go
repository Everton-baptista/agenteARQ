package policy_test

import (
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

var assessNow = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

// conformantAgent is an agent that satisfies every level, which each test then breaks in
// exactly one way.
func conformantAgent() policy.AgentEvidence {
	return policy.AgentEvidence{
		ID: "a",
		Manifest: map[string]any{"agent": map[string]any{
			"id":           "a",
			"owner":        map[string]any{"accountable": "alex.moreau"},
			"out_of_scope": []any{"never issues refunds"},
			"autonomy": map[string]any{
				"level":           "L1_act_with_approval",
				"stop_conditions": []any{"answer delivered"},
			},
			"guardrails": map[string]any{
				"input": []any{}, "output": []any{}, "action": []any{},
			},
		}},
		EvalCompletedAt: "2026-07-20",
		EvalMaxAgeDays:  30,
		RedTeamExecuted: true,
		HasThreatModel:  true,
		OTelEnabled:     true,
		SemconvPinned:   true,
	}
}

func assess(a policy.AgentEvidence) policy.Conformance {
	return policy.Assess([]policy.AgentEvidence{a}, true, true, assessNow)
}

func TestFullyConformantProjectReachesL3(t *testing.T) {
	c := assess(conformantAgent())
	if c.Level != policy.LevelL3 {
		for _, r := range c.Requirements {
			if !r.Met {
				t.Logf("unmet: %s — %s %s", r.ID, r.Text, r.Details)
			}
		}
		t.Fatalf("level = %s, want L3", c.Level)
	}
	if c.ExpiresAt == "" {
		t.Error("an L3 assessment must carry an expiry; conformance that never decays is advertising")
	}
}

// The mechanism that makes the badge honest: nobody downgrades it, time does.
func TestStaleEvalDropsL3ToL2(t *testing.T) {
	a := conformantAgent()
	a.EvalCompletedAt = "2026-05-01" // 88 days before, limit is 30

	c := assess(a)
	if c.Level != policy.LevelL2 {
		t.Fatalf("level = %s, want L2 — a stale eval must cost the L3 claim", c.Level)
	}
	if c.ExpiresAt != "" {
		t.Error("only an L3 assessment carries an expiry")
	}
	found := false
	for _, r := range c.Requirements {
		if r.ID == "L3-FRESH" && !r.Met {
			found = true
			if r.Details == "" {
				t.Error("the miss should say how stale the result is")
			}
		}
	}
	if !found {
		t.Error("L3-FRESH should be the requirement that failed")
	}
}

// An evaluation that was never run is not the same as one that lapsed, and neither counts.
func TestMissingEvalBlocksL3(t *testing.T) {
	a := conformantAgent()
	a.EvalCompletedAt = ""
	if c := assess(a); c.Level != policy.LevelL2 {
		t.Fatalf("level = %s, want L2", c.Level)
	}
}

// A gate that exists only on a laptop is a linter. The claim of L2 is that the rules block a
// merge, which requires them to run where merges happen.
func TestGateNotInCIBlocksL2(t *testing.T) {
	c := policy.Assess([]policy.AgentEvidence{conformantAgent()}, false, true, assessNow)
	if c.Level != policy.LevelL1 {
		t.Fatalf("level = %s, want L1", c.Level)
	}
}

func TestDriftedShimsBlockL1(t *testing.T) {
	c := policy.Assess([]policy.AgentEvidence{conformantAgent()}, true, false, assessNow)
	if c.Level != policy.LevelNone {
		t.Fatalf("level = %s, want none — out-of-date instruction files mean the assistants "+
			"are not being told the rules the project claims to follow", c.Level)
	}
}

func TestFailingBlockerBlocksL2(t *testing.T) {
	a := conformantAgent()
	a.Results = []policy.Result{{
		ControlID: "control.ai.tool.least_privilege",
		Severity:  policy.SevBlocker, Passed: false,
	}}
	if c := assess(a); c.Level != policy.LevelL1 {
		t.Fatalf("level = %s, want L1", c.Level)
	}
}

// A missing guardrail key is different from an empty one: empty records a decision, absent
// records an oversight.
func TestMissingGuardrailPointBlocksL2(t *testing.T) {
	a := conformantAgent()
	ag, _ := a.Manifest["agent"].(map[string]any)
	ag["guardrails"] = map[string]any{"input": []any{}, "output": []any{}} // no action

	if c := assess(a); c.Level != policy.LevelL1 {
		t.Fatalf("level = %s, want L1", c.Level)
	}
}

func TestNoAgentsIsNotConformant(t *testing.T) {
	if c := policy.Assess(nil, true, true, assessNow); c.Level != policy.LevelNone {
		t.Fatalf("level = %s, want none", c.Level)
	}
}

func TestBadgeReflectsLevel(t *testing.T) {
	cases := map[policy.Level]string{
		policy.LevelL3: "brightgreen", policy.LevelL2: "green",
		policy.LevelL1: "yellow", policy.LevelNone: "red",
	}
	for lvl, colour := range cases {
		b := policy.Conformance{Level: lvl}.Badge()
		if b["color"] != colour {
			t.Errorf("%s badge colour = %v, want %s", lvl, b["color"], colour)
		}
		if b["schemaVersion"] != 1 {
			t.Errorf("badge must be a shields.io endpoint document")
		}
	}
}

// Score must keep the two apart. Collapsing them is how "we are compliant" comes to mean
// nothing: a project can satisfy every declared control and have changed nothing real.
func TestScoreSeparatesDeclaredFromProven(t *testing.T) {
	results := []policy.Result{
		{ControlID: "control.ai.agent.owner_defined", Passed: true, Evidence: []string{"manifest_field"}},
		{ControlID: "control.ai.agent.scope_declared", Passed: true, Evidence: []string{"manifest_field"}},
		{ControlID: "control.ai.tool.least_privilege", Passed: false, Evidence: []string{"tool_spec", "test_result"}},
	}
	dims := policy.Score(results)

	byName := map[string]policy.Dimension{}
	for _, d := range dims {
		byName[d.Name] = d
	}
	if got := byName["agent"].Declared; got != 100 {
		t.Errorf("agent declared = %.0f, want 100", got)
	}
	if got := byName["tool"].Proven; got != 0 {
		t.Errorf("tool proven = %.0f, want 0 — the failing control rests on an artifact", got)
	}
	if byName["agent"].Proven != 0 {
		t.Error("a manifest-only control must not count towards proven")
	}
}
