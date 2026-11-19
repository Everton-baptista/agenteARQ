package policy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
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

// evaluateWith runs the standard profile's api.edge controls against one agent context and
// returns the results keyed by control id.
func evaluateWith(t *testing.T, agent map[string]any) map[string]policy.Result {
	t.Helper()
	cat := catalog(t)
	res, _, err := policy.ResolvePacks(cat, []string{"api.edge"}, agent)
	if err != nil {
		t.Fatal(err)
	}
	ctx := map[string]any{"agent": agent, "tools": []any{}, "evals": map[string]any{}}
	out := map[string]policy.Result{}
	for _, r := range policy.Evaluate(cat, res, ctx, nil, time.Now().UTC()) {
		out[r.ControlID] = r
	}
	return out
}

// An agent nobody can call over HTTP was being told it had not declared who may call it over
// HTTP. Four findings about a service that does not exist, on a project whose only defect was a
// floating model alias — and the noise is what made a CI assertion read the wrong result first.
func TestServiceControlsSkipAnAgentWithNoInterface(t *testing.T) {
	got := evaluateWith(t, agentCtx(nil)["agent"].(map[string]any))

	for _, id := range []string{
		"control.ai.api.caller_identified",
		"control.ai.api.request_logging_redacted",
		"control.ai.api.budget_per_caller",
		"control.ai.api.contract_generated",
		"control.ai.api.core_transport_separated",
	} {
		r, ok := got[id]
		if !ok {
			t.Errorf("%s was not resolved at all", id)
			continue
		}
		if !r.Skipped {
			t.Errorf("%s was evaluated against an agent with no interface", id)
		}
	}
}

// The two blockers read the repository, not the interface. A committed credential is public in a
// library and a batch job exactly as it is in a service, and scoping them to an interface would
// be the same defect as noise with the sign reversed: a control quietly not running where it was
// needed.
func TestRepositoryBlockersApplyWithoutAnInterface(t *testing.T) {
	got := evaluateWith(t, agentCtx(nil)["agent"].(map[string]any))

	for _, id := range []string{
		"control.ai.api.secrets_not_committed",
		"control.ai.api.no_client_side_model_access",
	} {
		r, ok := got[id]
		if !ok {
			t.Fatalf("%s was not resolved", id)
		}
		if r.Skipped {
			t.Errorf("%s skipped itself on an agent with no interface; it reads the "+
				"repository, and a committed credential is public everywhere", id)
		}
	}
}

// Declaring an interface brings the whole pack back. Without this the previous two tests are
// satisfied by a pack that never runs.
func TestServiceControlsRunOnceAnInterfaceIsDeclared(t *testing.T) {
	agent := agentCtx(map[string]any{
		"interface": map[string]any{
			"base_path": "/v1",
			"caller":    map[string]any{"identified_by": "bearer_token"},
		},
	})["agent"].(map[string]any)

	got := evaluateWith(t, agent)

	r, ok := got["control.ai.api.caller_identified"]
	if !ok {
		t.Fatal("caller_identified was not resolved")
	}
	if r.Skipped {
		t.Fatal("caller_identified skipped an agent that declares an interface")
	}
	if !r.Passed {
		t.Errorf("caller_identified failed an agent declaring identified_by: %s", r.Message)
	}
}

// exists() treats an empty map as absent, and applicability must agree with it. Otherwise
// `interface: {}` is a way to make a control run with nothing for it to read — or to make it
// skip while a check would have seen a value.
func TestAnEmptyInterfaceCountsAsUndeclared(t *testing.T) {
	agent := agentCtx(map[string]any{"interface": map[string]any{}})["agent"].(map[string]any)

	got := evaluateWith(t, agent)
	if r := got["control.ai.api.caller_identified"]; !r.Skipped {
		t.Error("interface: {} declares no interface; applicability must match exists()")
	}
}

// applies_to.declares is a closed vocabulary, not a path language. A second evaluation surface
// reachable by a third-party pack, running ahead of the checks the expression language was
// written to constrain, is the thing spec/normative/04 exists to prevent.
func TestDeclaresNamesOnlyOptionalManifestSections(t *testing.T) {
	required := map[string]bool{
		"id": true, "owner": true, "stage": true, "system_type": true, "purpose": true,
		"out_of_scope": true, "autonomy": true, "model": true, "prompts": true,
		"guardrails": true,
	}
	known := map[string]bool{
		"context": true, "evaluation": true, "handoff": true, "interface": true,
		"jurisdictions": true, "languages": true, "lifecycle": true, "links": true,
		"mcp": true, "observability": true, "policy": true, "privacy": true,
		"tools": true, "users": true,
	}

	cat := catalog(t)
	for id, c := range cat.Controls {
		for _, s := range c.AppliesTo.Declares {
			if required[s] {
				t.Errorf("%s declares %q, which the manifest requires — the condition can "+
					"never skip, so the control only looks conditional", id, s)
			}
			if !known[s] {
				t.Errorf("%s declares %q, which is not a manifest section", id, s)
			}
		}
	}
}

// deprecatedCatalog is a fixture pack holding one deprecated control and one stable one. No
// shipped control is deprecated yet, and marking one just to have an example would be a lie in
// the catalogue — so the behaviour is pinned against a fixture instead.
func deprecatedCatalog(t *testing.T) *policy.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"packs/fixture.deprecation/pack.yaml": {Data: []byte(`
schema_version: "1.0"
pack:
  id: fixture.deprecation
  version: 1.0.0
  title: Fixture
  authority_status: best_practice
  reviewed_at: "2026-07-30"
  requires:
    - control: control.ai.agent.owner_defined
      severity: blocker
    - control: control.ai.agent.scope_declared
      severity: major
`)},
		"packs/controls/agent/owner_defined.yaml": {Data: []byte(`
schema_version: "1.0"
control:
  id: control.ai.agent.owner_defined
  type: agent
  title: An accountable person is named
  intent: A queue is not accountable.
  status: deprecated
  check:
    kind: static_manifest
    expr: agent.owner.accountable != null
    message: No accountable person is named.
  evidence: [manifest_field]
  remediation: Name a person.
  standard_ref: standards/01-agent-contract.md#owner
`)},
		"packs/controls/agent/scope_declared.yaml": {Data: []byte(`
schema_version: "1.0"
control:
  id: control.ai.agent.scope_declared
  type: agent
  title: Out of scope is declared
  intent: An agent never told what to refuse will attempt it.
  status: stable
  check:
    kind: static_manifest
    expr: exists(agent.out_of_scope)
    message: Nothing declared out of scope.
  evidence: [manifest_field]
  remediation: Declare at least one entry.
  standard_ref: standards/01-agent-contract.md#scope
`)},
	}
	cat, err := policy.LoadCatalog(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// spec/normative/08-versioning.md: "A control marked deprecated MUST still be evaluated and MUST
// be reported as deprecated. It MUST NOT be silently dropped: a project relying on it deserves to
// be told, and a control that disappears without notice looks like a fixed failure."
//
// The status was parsed onto Control and read by nothing but `explain`, so the gate, the report
// and the SARIF output all said the same thing about a deprecated control as about a live one.
func TestDeprecatedControlIsStillEvaluatedAndSaysSo(t *testing.T) {
	cat := deprecatedCatalog(t)
	agent := map[string]any{
		"id": "t", "system_type": "agentic_task", "stage": "pilot",
		"autonomy": map[string]any{"level": "L2_act_reversible"},
		// Both controls fail: the deprecated one must be reported failing, not dropped.
	}
	res, _, err := policy.ResolvePacks(cat, []string{"fixture.deprecation"}, agent)
	if err != nil {
		t.Fatal(err)
	}
	ctx := map[string]any{"agent": agent, "tools": []any{}, "evals": map[string]any{}}

	byID := map[string]policy.Result{}
	for _, r := range policy.Evaluate(cat, res, ctx, nil, time.Now().UTC()) {
		byID[r.ControlID] = r
	}

	dep, ok := byID["control.ai.agent.owner_defined"]
	if !ok {
		t.Fatal("the deprecated control was dropped; it must still be evaluated")
	}
	if dep.Passed || dep.Skipped {
		t.Error("the deprecated control was not evaluated against the manifest")
	}
	if !dep.Deprecated {
		t.Error("the deprecated control is not reported as deprecated, so a project relying " +
			"on it has no warning before it is removed")
	}

	live, ok := byID["control.ai.agent.scope_declared"]
	if !ok {
		t.Fatal("the stable control was not evaluated")
	}
	if live.Deprecated {
		t.Error("a stable control is reported as deprecated")
	}
}
