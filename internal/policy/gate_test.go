package policy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func catalog(t *testing.T) *policy.Catalog {
	t.Helper()
	sub, err := fs.Sub(agentarch.Content, "content")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := policy.LoadCatalog(sub)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// Every control a pack requires must exist. A pack naming a control nobody defined looks
// rigorous and enforces nothing.
func TestEveryRequiredControlExists(t *testing.T) {
	cat := catalog(t)
	for _, p := range cat.Packs {
		for _, r := range p.Requires {
			if _, ok := cat.Controls[r.Control]; !ok {
				t.Errorf("pack %s requires %s, which is not in the catalogue", p.ID, r.Control)
			}
		}
	}
}

// Every control must parse. A malformed expression is reported as an error at runtime, but
// shipping one is a defect that belongs to the build.
func TestEveryControlExpressionParses(t *testing.T) {
	cat := catalog(t)
	if len(cat.Controls) == 0 {
		t.Fatal("no controls loaded")
	}
	for id, c := range cat.Controls {
		if c.Check.Expr == "" {
			continue
		}
		if _, err := policy.Parse(c.Check.Expr); err != nil {
			t.Errorf("%s has an unparseable expression: %v", id, err)
		}
	}
}

// Every control must point at a prose section, and every control must carry a remediation.
// A finding with no fix trains people to ignore findings.
func TestEveryControlIsDocumentedAndActionable(t *testing.T) {
	cat := catalog(t)
	for id, c := range cat.Controls {
		if c.StandardRef == "" {
			t.Errorf("%s has no standard_ref", id)
		}
		if len(c.Remediation) < 15 {
			t.Errorf("%s has no usable remediation", id)
		}
		if len(c.Intent) < 20 {
			t.Errorf("%s does not say why it exists", id)
		}
	}
}

// A pack whose blockers are all satisfied by filling in a manifest field is a form, not a
// standard.
//
// The rule applies from three blockers up. The failure mode being prevented is a pack that
// looks rigorous — many blockers — while satisfying all of them changes nothing real. A pack
// with one or two blockers cannot create that impression, and std.iso-42001 is the honest case:
// it certifies a management system, so most of its evidence is organisational and outside any
// repository. Forcing a synthetic executable check there would be the same dishonesty in the
// other direction.
func TestPackBlockersAreNotAllDeclarations(t *testing.T) {
	cat := catalog(t)
	for _, p := range cat.Packs {
		blockers, executable := 0, 0
		for _, r := range p.Requires {
			if r.Severity != policy.SevBlocker {
				continue
			}
			blockers++
			for _, e := range r.RequiredEvidence {
				if e != "manifest_field" {
					executable++
					break
				}
			}
		}
		if blockers >= 3 && executable == 0 {
			t.Errorf("pack %s has %d blockers, all evidenced by manifest_field alone — "+
				"that is a form to fill in, not a gate", p.ID, blockers)
		}
	}
}

func agentCtx(overrides map[string]any) map[string]any {
	agent := map[string]any{
		"id": "t", "system_type": "agentic_task", "stage": "pilot",
		"autonomy": map[string]any{"level": "L2_act_reversible"},
	}
	for k, v := range overrides {
		agent[k] = v
	}
	return map[string]any{"agent": agent, "tools": []any{}, "evals": map[string]any{}}
}

// The highest severity wins. Resolving downward would let an organisation pack quietly weaken
// a security pack it imported.
func TestResolutionTakesHighestSeverity(t *testing.T) {
	cat := catalog(t)
	ctx := agentCtx(nil)
	agent, _ := ctx["agent"].(map[string]any)

	res, _, err := policy.ResolvePacks(cat, []string{"core.agent", "sec.owasp-llm"}, agent)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res {
		if r.ControlID == "control.ai.tool.least_privilege" {
			found = true
			if r.Severity != policy.SevBlocker {
				t.Errorf("least_privilege resolved to %s, want blocker", r.Severity)
			}
		}
	}
	if !found {
		t.Error("least_privilege was not resolved from the standard profile")
	}
}

// A control inside its grace period runs in warn mode. A rule that starts failing the day it
// ships gets the whole gate switched off rather than getting anything fixed.
func TestGracePeriodDowngradesToWarn(t *testing.T) {
	cat := catalog(t)
	ctx := agentCtx(nil)
	agent, _ := ctx["agent"].(map[string]any)

	res, _, err := policy.ResolvePacks(cat, []string{"core.agent"}, agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.ControlID == "control.ai.genai.prompt_versioned" {
			if r.Severity != policy.SevWarn {
				t.Errorf("prompt_versioned is enforced_from a later version, so it should "+
					"resolve to warn; got %s", r.Severity)
			}
			return
		}
	}
	t.Error("prompt_versioned was not resolved")
}

func TestWaiverExpiry(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		w       policy.Waiver
		problem bool
	}{
		{"valid", policy.Waiver{Control: "c", Owner: "a.b", Reason: "r", Until: "2026-08-15"}, false},
		{"expired", policy.Waiver{Control: "c", Owner: "a.b", Reason: "r", Until: "2026-06-01"}, true},
		{"no owner", policy.Waiver{Control: "c", Reason: "r", Until: "2026-08-15"}, true},
		{"no reason", policy.Waiver{Control: "c", Owner: "a.b", Until: "2026-08-15"}, true},
		{"no expiry", policy.Waiver{Control: "c", Owner: "a.b", Reason: "r"}, true},
		{"too far out", policy.Waiver{Control: "c", Owner: "a.b", Reason: "r", Until: "2027-12-31"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := policy.CheckWaivers([]policy.Waiver{tc.w}, now)
			if tc.problem && len(got) == 0 {
				t.Errorf("waiver should have been rejected")
			}
			if !tc.problem && len(got) > 0 {
				t.Errorf("valid waiver was rejected: %s", got[0].Reason)
			}
		})
	}
}

// TestGateOnExamples runs the real gate against the example projects and asserts the outcome
// each expected.yaml documents.
func TestGateOnExamples(t *testing.T) {
	root := moduleRoot(t)
	cat := catalog(t)
	now := time.Now().UTC()

	type want struct {
		dir      string
		blockers int
	}
	cases := []want{
		{filepath.Join(root, "examples", "01-rag-support-agent"), 0},
		{filepath.Join(root, "examples", "99-failing", "unpinned-model"), 1},
		{filepath.Join(root, "examples", "99-failing", "autonomous-irreversible-unapproved"), 1},
	}

	for _, c := range cases {
		t.Run(filepath.Base(c.dir), func(t *testing.T) {
			if _, err := os.Stat(c.dir); err != nil {
				t.Skip("example not present")
			}
			agents, err := policy.LoadAgents(c.dir, now)
			if err != nil {
				t.Fatal(err)
			}
			total := 0
			for _, a := range agents {
				agent, _ := a.Ctx["agent"].(map[string]any)
				res, _, err := policy.ResolvePacks(cat, policy.PacksFor("standard", agent), agent)
				if err != nil {
					t.Fatal(err)
				}
				out := policy.Evaluate(cat, res, a.Ctx, nil, now)
				sum := policy.Summarize(out)
				if len(sum.Errors) > 0 {
					t.Fatalf("control could not be evaluated: %s — %s",
						sum.Errors[0].ControlID, sum.Errors[0].Error)
				}
				total += len(sum.Blockers)
			}
			if total != c.blockers {
				t.Errorf("got %d blocker(s), want %d", total, c.blockers)
			}
		})
	}
}
