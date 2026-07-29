package emit

import (
	"strings"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func sample(blockers bool) Report {
	results := []policy.Result{
		{AgentID: "a", ControlID: "control.ai.agent.owner_defined", Passed: true,
			Severity: policy.SevBlocker, Evidence: []string{"manifest_field"}},
		{AgentID: "a", ControlID: "control.ai.tool.least_privilege", Passed: false,
			Severity: policy.SevBlocker, Message: "A tool does not deny egress by default.",
			Remediation: "Set deny_by_default: true.", Evidence: []string{"tool_spec"}},
	}
	if !blockers {
		results[1].Passed = true
		results[1].Message = ""
	}
	return Report{
		Project: "demo", Profile: "standard", Version: "0.1.0",
		GeneratedAt: time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
		Results:     results,
		Summary:     policy.Summarize(results),
		Conformance: policy.Conformance{Level: policy.LevelL2, Requirements: []policy.Requirement{
			{Level: policy.LevelL1, ID: "L1-OWNER", Text: "every agent names an owner", Met: true},
			{Level: policy.LevelL3, ID: "L3-EVAL", Text: "every agent has an eval", Met: false,
				Details: "a"},
		}},
		Dimensions: policy.Score(results),
	}
}

func render(t *testing.T, r Report, html bool) string {
	t.Helper()
	var b strings.Builder
	var err error
	if html {
		err = HTML(&b, r)
	} else {
		err = Markdown(&b, r)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// A report that opens with a percentage invites the reader to treat the number as the finding,
// and the number is the least actionable thing in it.
func TestReportLeadsWithWhatIsWrong(t *testing.T) {
	out := render(t, sample(true), false)
	blocked := strings.Index(out, "## Blocked")
	maturity := strings.Index(out, "## Maturity")
	if blocked < 0 {
		t.Fatal("a blocked project should say so")
	}
	if maturity > 0 && blocked > maturity {
		t.Error("the failing controls must come before the score")
	}
	if !strings.Contains(out, "Set deny_by_default") {
		t.Error("a finding without its remediation trains people to ignore findings")
	}
}

func TestCleanReportSaysSoPlainly(t *testing.T) {
	out := render(t, sample(false), false)
	if !strings.Contains(out, "Not blocked") {
		t.Error("a clean project should be told plainly, not left to infer it")
	}
	if strings.Contains(out, "## Blocked") {
		t.Error("a clean report must not have a Blocked section")
	}
}

// Conformance without its expiry reads as permanent.
func TestExpiryIsShownWhenPresent(t *testing.T) {
	r := sample(false)
	r.Conformance.Level = policy.LevelL3
	r.Conformance.ExpiresAt = "2026-08-27"
	out := render(t, r, false)
	if !strings.Contains(out, "2026-08-27") || !strings.Contains(out, "drops to L2") {
		t.Error("an L3 report must show when it stops being true")
	}
}

func TestDeclaredAndProvenStayApart(t *testing.T) {
	out := render(t, sample(false), false)
	if !strings.Contains(out, "Declared") || !strings.Contains(out, "Proven") {
		t.Fatal("the two must be separate columns")
	}
	if !strings.Contains(out, "comes to mean nothing") {
		t.Error("the report should say why they are kept apart")
	}
}

// A report is read from a filesystem, an artifact store, or an air-gapped machine.
func TestHTMLIsSelfContained(t *testing.T) {
	out := render(t, sample(true), true)
	for _, forbidden := range []string{"<script", "src=\"http", "href=\"http", "@import"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("HTML report contains %q; it must make no external request", forbidden)
		}
	}
	if !strings.Contains(out, "<!doctype html>") || !strings.Contains(out, "</html>") {
		t.Error("HTML report is not a complete document")
	}
}

func TestHTMLTagsAreBalanced(t *testing.T) {
	out := render(t, sample(true), true)
	for _, pair := range [][2]string{
		{"<table>", "</table>"}, {"<tr>", "</tr>"}, {"<h2>", "</h2>"}, {"<code>", "</code>"},
	} {
		if strings.Count(out, pair[0]) != strings.Count(out, pair[1]) {
			t.Errorf("unbalanced %s: %d open, %d close",
				pair[0], strings.Count(out, pair[0]), strings.Count(out, pair[1]))
		}
	}
}

// A control id or a message could contain markup. Escaping has to happen before the report's own
// tags are inserted, not after.
func TestHTMLEscapesContentButKeepsItsOwnMarkup(t *testing.T) {
	r := sample(true)
	r.Results[1].Message = `<script>alert("x")</script> & "quoted"`
	r.Summary = policy.Summarize(r.Results)

	out := render(t, r, true)
	if strings.Contains(out, "<script>alert") {
		t.Fatal("content was not escaped")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the escaped form to appear")
	}
	if !strings.Contains(out, "<td>") {
		t.Error("the report's own markup was escaped away")
	}
}

func TestWaiverProblemsAreSurfaced(t *testing.T) {
	r := sample(false)
	r.WaiverIssue = []policy.WaiverProblem{{
		Waiver: policy.Waiver{Control: "control.ai.x.y", Owner: "a.person"},
		Reason: "expired on 2026-01-01",
	}}
	out := render(t, r, false)
	if !strings.Contains(out, "expired on 2026-01-01") || !strings.Contains(out, "a.person") {
		t.Error("a lapsed waiver must name its reason and its owner")
	}
}
