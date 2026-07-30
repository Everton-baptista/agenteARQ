package emit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func emitSARIF(t *testing.T, results []policy.Result) sarifLog {
	t.Helper()
	var b strings.Builder
	if err := SARIF(&b, results, "0.2.0", func(policy.Result) string { return "agentarch.yaml" }); err != nil {
		t.Fatal(err)
	}
	var log sarifLog
	if err := json.Unmarshal([]byte(b.String()), &log); err != nil {
		t.Fatal(err)
	}
	return log
}

func levels(log sarifLog) []string {
	var out []string
	for _, r := range log.Runs[0].Results {
		out = append(out, r.Level)
	}
	return out
}

// Ordering by control ID put whichever rule sorted earliest at the top, which in a project
// carrying warn-mode controls meant a "note" led the file while the blocker was buried. Anything
// reading only the first result — a CI assertion, a truncated annotation list — was told the
// build was fine.
func TestBlockerLeadsTheSARIFResults(t *testing.T) {
	results := []policy.Result{
		{AgentID: "a", ControlID: "control.ai.api.budget_per_caller", Severity: policy.SevWarn,
			Message: "No per-caller budget is declared."},
		{AgentID: "a", ControlID: "control.ai.api.caller_identified", Severity: policy.SevWarn,
			Message: "No caller identification is declared."},
		{AgentID: "a", ControlID: "control.ai.genai.output_guardrail", Severity: policy.SevMajor,
			Message: "No output guardrail is declared."},
		{AgentID: "a", ControlID: "control.ai.supply.model_pinned", Severity: policy.SevBlocker,
			Message: "The model is a floating alias."},
	}

	log := emitSARIF(t, results)
	got := log.Runs[0].Results
	if len(got) != 4 {
		t.Fatalf("expected 4 results, got %d", len(got))
	}
	if got[0].Level != "error" {
		t.Errorf("the first result is %q; a reader who sees only one must see the blocker", got[0].Level)
	}
	if got[0].RuleID != "control.ai.supply.model_pinned" {
		t.Errorf("first result is %s, want the blocker", got[0].RuleID)
	}

	want := []string{"error", "warning", "note", "note"}
	for i, lvl := range levels(log) {
		if lvl != want[i] {
			t.Errorf("result %d is %q, want %q — results must descend by severity", i, lvl, want[i])
		}
	}
}

// Two findings at the same level must not swap places between runs, or a diff of two SARIF files
// reports churn that is not there.
func TestSARIFOrderIsDeterministicWithinALevel(t *testing.T) {
	results := []policy.Result{
		{AgentID: "a", ControlID: "control.ai.tool.timeout_declared", Severity: policy.SevBlocker},
		{AgentID: "a", ControlID: "control.ai.agent.owner_defined", Severity: policy.SevBlocker},
		{AgentID: "a", ControlID: "control.ai.mcp.allowlist_enforced", Severity: policy.SevBlocker},
	}

	log := emitSARIF(t, results)
	var ids []string
	for _, r := range log.Runs[0].Results {
		ids = append(ids, r.RuleID)
	}
	want := []string{
		"control.ai.agent.owner_defined",
		"control.ai.mcp.allowlist_enforced",
		"control.ai.tool.timeout_declared",
	}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("result %d is %s, want %s — equal severities tie-break on control ID", i, id, want[i])
		}
	}
}

// A waived blocker reports as a note, so it must sort where it reads. Ranking on severity rather
// than on the emitted level would float it to the top of a file where it appears harmless.
func TestWaivedBlockerSortsAsTheNoteItReportsAs(t *testing.T) {
	results := []policy.Result{
		{AgentID: "a", ControlID: "control.ai.agent.owner_defined", Severity: policy.SevBlocker,
			Waived: true, WaiverUntil: "2026-09-01", WaiverOwner: "a.person"},
		{AgentID: "a", ControlID: "control.ai.supply.model_pinned", Severity: policy.SevBlocker},
	}

	log := emitSARIF(t, results)
	got := log.Runs[0].Results
	if got[0].RuleID != "control.ai.supply.model_pinned" {
		t.Errorf("first result is %s; the unwaived blocker is the one that blocks", got[0].RuleID)
	}
	if got[1].Level != "note" {
		t.Errorf("waived blocker emitted as %q, want note", got[1].Level)
	}
}

// Passing controls are not findings. Emitting them would bury the failures the file exists for.
func TestSARIFOmitsPassingControls(t *testing.T) {
	results := []policy.Result{
		{AgentID: "a", ControlID: "control.ai.agent.owner_defined", Severity: policy.SevBlocker, Passed: true},
		{AgentID: "a", ControlID: "control.ai.supply.model_pinned", Severity: policy.SevBlocker},
	}

	log := emitSARIF(t, results)
	if n := len(log.Runs[0].Results); n != 1 {
		t.Fatalf("expected 1 result, got %d", n)
	}
	if log.Runs[0].Results[0].RuleID != "control.ai.supply.model_pinned" {
		t.Error("the wrong control was emitted")
	}
}
