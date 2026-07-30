package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func iface() InterfaceOf {
	return InterfaceOf{
		AgentID:   "support-triage",
		Purpose:   "answers from a corpus",
		Transport: "http",
		BasePath:  "/v1",
		Auth:      "bearer_jwt",
		Routes: []Route{
			{Path: "/ask", Method: "POST", Summary: "ask", AuthRequired: true},
			{Path: "/health", Method: "GET", Summary: "readiness", AuthRequired: false, Idempotent: true},
		},
	}
}

func TestOpenAPIIsDeterministic(t *testing.T) {
	// Go map ordering is deliberately random. A generated file that differs between two runs makes
	// sync --check report drift that never happened, and the fix people reach for is to stop running it.
	a, _, err := BuildOpenAPI(iface(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		b, _, err := BuildOpenAPI(iface(), "1.0.0")
		if err != nil {
			t.Fatal(err)
		}
		if string(a) != string(b) {
			t.Fatal("two renders of the same interface differ")
		}
	}
}

func TestTheDigestCoversTheContractAndNotTheWording(t *testing.T) {
	_, base, _ := BuildOpenAPI(iface(), "1.0.0")

	reworded := iface()
	reworded.Routes[0].Summary = "ask the agent a question, politely"
	_, same, _ := BuildOpenAPI(reworded, "1.0.0")
	if same != base {
		t.Error("a summary rewording changed the digest; it should not be an interface change")
	}

	added := iface()
	added.Routes = append(added.Routes, Route{Path: "/purge", Method: "DELETE", AuthRequired: true})
	_, changed, _ := BuildOpenAPI(added, "1.0.0")
	if changed == base {
		t.Error("adding a route did not change the digest")
	}

	opened := iface()
	opened.Routes[0].AuthRequired = false
	_, unauth, _ := BuildOpenAPI(opened, "1.0.0")
	if unauth == base {
		t.Error("removing auth from a route did not change the digest — the one change most worth noticing")
	}
}

func TestAuthenticatedRoutesDeclareTheirSchemeAndTheirRefusals(t *testing.T) {
	out, _, _ := BuildOpenAPI(iface(), "1.0.0")
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	ask := paths["/v1/ask"].(map[string]any)["post"].(map[string]any)

	sec, _ := ask["security"].([]any)
	if len(sec) != 1 {
		t.Errorf("an authenticated route should name its scheme, got %v", ask["security"])
	}
	resp := ask["responses"].(map[string]any)
	for _, code := range []string{"401", "429"} {
		if _, ok := resp[code]; !ok {
			t.Errorf("missing %s on an authenticated route", code)
		}
	}

	// A public route must be visibly public rather than inheriting a document-level default. A global
	// security block plus one exception is how an endpoint ends up open without anyone noticing.
	health := paths["/v1/health"].(map[string]any)["get"].(map[string]any)
	if s, _ := health["security"].([]any); len(s) != 0 {
		t.Errorf("the public route should declare empty security, got %v", health["security"])
	}
}

func TestNonIdempotentRoutesAreMarked(t *testing.T) {
	// A caller that retries a non-idempotent call performs it twice, and for an irreversible action
	// that is the whole failure.
	out, _, _ := BuildOpenAPI(iface(), "1.0.0")
	if !strings.Contains(string(out), `"x-agentarch-idempotent": false`) {
		t.Error("a non-idempotent route was not marked")
	}
}

func TestTheGeneratedFileSaysItIsGenerated(t *testing.T) {
	out, digest, _ := BuildOpenAPI(iface(), "1.0.0")
	if !strings.Contains(string(out), "DO NOT EDIT") {
		t.Error("a generated file that does not say so gets hand-edited")
	}
	if SourceDigestOf(out) != digest {
		t.Error("the recorded digest is not the one returned")
	}
	if SourceDigestOf([]byte(`{"openapi":"3.1.0"}`)) != "" {
		t.Error("a hand-written contract should report no digest, so it is distinguishable")
	}
}

func TestARefusalIsBetterThanAnEmptyContract(t *testing.T) {
	noRoutes := iface()
	noRoutes.Routes = nil
	if _, _, err := BuildOpenAPI(noRoutes, "1.0.0"); err == nil {
		t.Error("an interface with no routes should refuse rather than describe nothing")
	}

	lib := iface()
	lib.Transport = "library"
	_, _, err := BuildOpenAPI(lib, "1.0.0")
	if err == nil || !strings.Contains(err.Error(), "no HTTP contract") {
		t.Errorf("a library agent has no endpoints; got %v", err)
	}
}
