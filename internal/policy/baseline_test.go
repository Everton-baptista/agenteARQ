package policy_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

var bNow = time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)

func fail(agent, control string, sev policy.Severity) policy.Result {
	return policy.Result{AgentID: agent, ControlID: control, Severity: sev, Passed: false}
}

func pass(agent, control string) policy.Result {
	return policy.Result{AgentID: agent, ControlID: control, Passed: true}
}

// The whole point: a project with existing agents fails on day one, and without a way in that
// costs nothing that day, the gate gets switched off and nothing improves.
func TestBaselineLetsExistingFailuresThrough(t *testing.T) {
	results := []policy.Result{
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker),
		fail("a", "control.ai.genai.output_guardrail", policy.SevBlocker),
		pass("a", "control.ai.agent.owner_defined"),
	}
	b := policy.NewBaseline(results, "standard", bNow)
	if len(b.Accepted) != 2 {
		t.Fatalf("expected 2 baselined failures, got %d", len(b.Accepted))
	}

	sum := policy.Summarize(policy.ApplyBaseline(results, b))
	if len(sum.Blockers) != 0 {
		t.Fatalf("baselined failures must not block, got %d", len(sum.Blockers))
	}
	if sum.Baselined != 2 {
		t.Errorf("baselined count = %d, want 2", sum.Baselined)
	}
	// Not counted as passing. A ratchet that hides its debt is an amnesty.
	if sum.Passed != 1 {
		t.Errorf("passed = %d, want 1 — a baselined failure is not a pass", sum.Passed)
	}
}

// The ratchet only turns one way.
func TestANewFailureStillBlocks(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker),
	}, "standard", bNow)

	later := []policy.Result{
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker), // known
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),  // new
	}
	sum := policy.Summarize(policy.ApplyBaseline(later, b))
	if len(sum.Blockers) != 1 {
		t.Fatalf("a new failure must block, got %d blocker(s)", len(sum.Blockers))
	}
	if sum.Blockers[0].ControlID != "control.ai.supply.model_pinned" {
		t.Errorf("the wrong failure blocked: %s", sum.Blockers[0].ControlID)
	}
}

// A failure on an agent added after adoption was never baselined, whatever its control.
func TestFailureOnANewAgentBlocks(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("old", "control.ai.tool.least_privilege", policy.SevBlocker),
	}, "standard", bNow)

	sum := policy.Summarize(policy.ApplyBaseline([]policy.Result{
		fail("old", "control.ai.tool.least_privilege", policy.SevBlocker),
		fail("new", "control.ai.tool.least_privilege", policy.SevBlocker),
	}, b))

	if len(sum.Blockers) != 1 || sum.Blockers[0].AgentID != "new" {
		t.Fatalf("the new agent's failure must block, got %v", sum.Blockers)
	}
}

// Adopting a baseline must not grandfather a severity that did not exist yet.
func TestRaisedSeverityIsNotCovered(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.tool.timeout_declared", policy.SevMajor),
	}, "standard", bNow)

	sum := policy.Summarize(policy.ApplyBaseline([]policy.Result{
		fail("a", "control.ai.tool.timeout_declared", policy.SevBlocker),
	}, b))

	if len(sum.Blockers) != 1 {
		t.Fatal("a control that became stricter must not stay covered by the old baseline")
	}
}

func TestUpdateRemovesFixedEntries(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker),
	}, "standard", bNow)

	next := b.Update([]policy.Result{
		pass("a", "control.ai.supply.model_pinned"),                     // fixed
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker), // still open
	}, bNow)

	if len(next.Accepted) != 1 {
		t.Fatalf("expected 1 entry after update, got %d", len(next.Accepted))
	}
	if next.CreatedAt != b.CreatedAt {
		t.Error("the original adoption date should be preserved")
	}
	if next.UpdatedAt == "" {
		t.Error("an update should record when it happened")
	}
}

// Update must never add. Adding on update would let a regression enter the baseline the moment
// it appeared — exactly the amnesty this exists to avoid.
func TestUpdateNeverAdds(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),
	}, "standard", bNow)

	next := b.Update([]policy.Result{
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker), // regression
	}, bNow)

	if len(next.Accepted) != 1 {
		t.Fatalf("update must not absorb a new failure; got %d entries", len(next.Accepted))
	}
}

func TestDiffPartitionsCorrectly(t *testing.T) {
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker),
		fail("gone", "control.ai.agent.owner_defined", policy.SevBlocker),
	}, "standard", bNow)

	d := b.Diff([]policy.Result{
		pass("a", "control.ai.supply.model_pinned"),
		fail("a", "control.ai.tool.least_privilege", policy.SevBlocker),
		fail("a", "control.ai.eval.result_fresh", policy.SevBlocker),
	})

	if len(d.Fixed) != 1 || len(d.StillOpen) != 1 || len(d.Stale) != 1 || len(d.NewFailure) != 1 {
		t.Fatalf("fixed=%d open=%d stale=%d new=%d",
			len(d.Fixed), len(d.StillOpen), len(d.Stale), len(d.NewFailure))
	}
}

func TestNoBaselineMeansEverythingIsNew(t *testing.T) {
	var b *policy.Baseline
	d := b.Diff([]policy.Result{fail("a", "control.ai.x.y", policy.SevBlocker)})
	if len(d.NewFailure) != 1 {
		t.Fatal("with no baseline, every failure is new")
	}
	// And applying a nil baseline changes nothing.
	in := []policy.Result{fail("a", "control.ai.x.y", policy.SevBlocker)}
	if len(policy.Summarize(policy.ApplyBaseline(in, nil)).Blockers) != 1 {
		t.Fatal("a nil baseline must not suppress anything")
	}
}

func TestRoundTripThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b := policy.NewBaseline([]policy.Result{
		fail("a", "control.ai.supply.model_pinned", policy.SevBlocker),
	}, "standard", bNow)

	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := policy.LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Accepted) != 1 || back.Profile != "standard" {
		t.Fatalf("round trip lost data: %+v", back)
	}

	missing, err := policy.LoadBaseline(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || missing != nil {
		t.Fatal("a missing baseline is not an error; most projects do not need one")
	}
}

// A skipped control was not evaluated, so it is neither a pass nor a failure — spec/normative/
// 02-control-and-pack.md §4. Folding it into the pass count reports credit for a rule that never
// ran, and inflates the "N of M passed" line every reader uses as the summary.
func TestSkippedIsNeitherPassedNorCounted(t *testing.T) {
	sum := policy.Summarize([]policy.Result{
		{ControlID: "control.ai.api.caller_identified", Skipped: true, Passed: true,
			SkipReason: "the manifest declares no interface"},
		{ControlID: "control.ai.agent.owner_defined", Passed: true},
		{ControlID: "control.ai.supply.model_pinned", Severity: policy.SevBlocker},
	})

	if sum.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", sum.Skipped)
	}
	if sum.Passed != 1 {
		t.Errorf("passed = %d, want 1 — a skipped control is not a pass", sum.Passed)
	}
	if sum.Total != 2 {
		t.Errorf("total = %d, want 2 — a skipped control was not evaluated", sum.Total)
	}
	if len(sum.Blockers) != 1 {
		t.Errorf("blockers = %d, want 1", len(sum.Blockers))
	}
}
