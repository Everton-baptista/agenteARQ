package policy_test

import (
	"testing"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func agentIn(jurisdictions []string, personalData bool) map[string]any {
	var js []any
	for _, j := range jurisdictions {
		js = append(js, j)
	}
	return map[string]any{
		"id": "a", "system_type": "agentic_task", "stage": "production",
		"jurisdictions": js,
		"autonomy":      map[string]any{"level": "L2_act_reversible"},
		"privacy":       map[string]any{"processes_personal_data": personalData},
	}
}

func resolvedFrom(t *testing.T, packs []string, agent map[string]any) map[string]string {
	t.Helper()
	res, _, err := policy.ResolvePacks(catalog(t), packs, agent)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range res {
		out[r.ControlID] = r.FromPack
	}
	return out
}

// The promise that makes an international standard adoptable: a team in Berlin, São Paulo and
// Austin share the same core, and only the obligations that actually apply to them resolve.
func TestJurisdictionPacksApplyOnlyWhereDeclared(t *testing.T) {
	all := []string{"core.agent", "reg.gdpr", "reg.br-lgpd", "reg.eu-ai-act"}

	eu := resolvedFrom(t, all, agentIn([]string{"EU"}, true))
	br := resolvedFrom(t, all, agentIn([]string{"BR"}, true))
	us := resolvedFrom(t, all, agentIn([]string{"US"}, true))

	if eu["control.ai.privacy.retention_declared"] == "" {
		t.Error("an EU agent processing personal data must pick up a retention obligation")
	}
	if br["control.ai.privacy.retention_declared"] == "" {
		t.Error("a BR agent processing personal data must pick up a retention obligation")
	}
	if us["control.ai.privacy.retention_declared"] != "" {
		t.Errorf("a US-only agent must not be judged by GDPR or LGPD; got it from %s",
			us["control.ai.privacy.retention_declared"])
	}

	// The AI Act applies on jurisdiction alone, not on personal-data processing.
	if eu["control.ai.eval.result_fresh"] == "" {
		t.Error("an EU agent should pick up the AI Act evaluation obligations")
	}
	if br["control.ai.eval.result_fresh"] != "" {
		t.Error("a BR-only agent must not be judged by the EU AI Act")
	}
}

// An agent operating in both places accumulates both sets. Union, never intersection —
// operating somewhere extra cannot remove an obligation.
func TestOperatingInTwoJurisdictionsAccumulates(t *testing.T) {
	both := resolvedFrom(t,
		[]string{"core.agent", "reg.gdpr", "reg.br-lgpd", "reg.eu-ai-act"},
		agentIn([]string{"EU", "BR"}, true))

	for _, id := range []string{
		"control.ai.privacy.retention_declared",
		"control.ai.eval.result_fresh",
		"control.ai.genai.prompt_versioned",
	} {
		if both[id] == "" {
			t.Errorf("%s should apply to an agent operating in both the EU and Brazil", id)
		}
	}
}

// A privacy pack does not fire for an agent that touches no personal data, wherever it runs.
func TestPrivacyPacksNeedPersonalData(t *testing.T) {
	noPD := resolvedFrom(t,
		[]string{"core.agent", "reg.gdpr"},
		agentIn([]string{"EU"}, false))

	if noPD["control.ai.privacy.retention_declared"] != "" {
		t.Error("GDPR must not impose a retention obligation on an agent processing no personal data")
	}
}

// A pack the project never selected imposes nothing, however severe its contents.
func TestUnselectedPackImposesNothing(t *testing.T) {
	only := resolvedFrom(t, []string{"core.agent"}, agentIn([]string{"EU"}, true))
	if only["control.ai.privacy.retention_declared"] != "" {
		t.Error("reg.gdpr was not selected, so none of its controls should resolve")
	}
}

// Binding law is never softened by a pack that happens to ask for less. Resolution takes the
// highest severity, so importing a lenient pack cannot weaken a legal obligation.
func TestBindingLawIsNotWeakenedByAnotherPack(t *testing.T) {
	cat := catalog(t)
	agent := agentIn([]string{"EU"}, true)

	res, _, err := policy.ResolvePacks(cat, []string{"core.agent", "reg.gdpr", "obs.otel"}, agent)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.ControlID == "control.ai.privacy.capture_content_default_off" {
			if r.Severity != policy.SevBlocker {
				t.Fatalf("capture_content_default_off resolved to %s; a binding-law obligation "+
					"must not be softened by another pack", r.Severity)
			}
			return
		}
	}
	t.Error("control was not resolved at all")
}

// Every pack states what its requirements rest on. This is what stops a voluntary framework or
// an internal preference from being presented to a team as a legal obligation.
func TestEveryPackDeclaresItsAuthority(t *testing.T) {
	valid := map[string]bool{
		"binding_law": true, "regulatory_instrument": true, "draft": true,
		"voluntary_standard": true, "best_practice": true,
	}
	for id, p := range catalog(t).Packs {
		if !valid[p.AuthorityStatus] {
			t.Errorf("pack %s has authority_status %q", id, p.AuthorityStatus)
		}
		if p.ReviewedAt == "" {
			t.Errorf("pack %s has no review date; external sources move faster than this package", id)
		}
		if p.AuthorityStatus == "binding_law" && len(p.Authority.Jurisdiction) == 0 {
			t.Errorf("pack %s claims binding law but names no jurisdiction", id)
		}
	}
}
