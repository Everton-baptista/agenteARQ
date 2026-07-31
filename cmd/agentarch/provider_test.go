package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestProviderTableIsReviewed is a deadline, and it is meant to be.
//
// The rest of agentarch is checked by its own rules; this one table is a set of facts about three
// other companies' products, and nothing in this repository can tell when one of them retires a
// model. A pinned id that has been discontinued is worse than a floating alias — it looks like a
// decision and is an oversight — so the only honest mechanism is a date somebody has to renew.
//
// When this fails: open each provider's model list, confirm every id and every SDK pin below, then
// move providersReviewed forward. Moving the date without doing the check is the one way to make
// this test worse than useless.
func TestProviderTableIsReviewed(t *testing.T) {
	age := time.Since(providersReviewed)
	if age > providerReviewInterval {
		t.Fatalf(
			"the provider table has not been reviewed for %d days (limit %d).\n"+
				"  cmd/agentarch/provider.go — confirm each ModelID and SDK against the provider's\n"+
				"  own documentation, then move providersReviewed to today.",
			int(age.Hours()/24), int(providerReviewInterval.Hours()/24))
	}
}

// Everything the table promises has to be there, because each field lands in a different file and
// a blank one fails somewhere far from here: an empty ModelID writes `id:` with no value and fails
// schema validation; an empty SDK writes a requirements line pip cannot parse.
func TestProviderTableIsComplete(t *testing.T) {
	for _, p := range providers {
		if p.ID == "" || p.Label == "" || p.ModelID == "" || p.SDK == "" || p.Key == "" {
			t.Errorf("provider %q has an empty field: %+v", p.ID, p)
		}
		if !strings.Contains(p.SDK, "==") {
			t.Errorf("provider %q pins %q, which is not an exact version", p.ID, p.SDK)
		}
		if !strings.HasSuffix(p.Key, "_API_KEY") {
			t.Errorf("provider %q names credential %q", p.ID, p.Key)
		}
	}
	if len(providers) == 0 || providers[0].ID != "anthropic" {
		t.Error("the first row is the default, and it must be what the blueprints already ship")
	}
}

// The seam in every blueprint carries the same three credential names. If the two lists drift, a
// project generated with --provider google would look for a variable the code never reads.
func TestCredentialNamesMatchTheSeam(t *testing.T) {
	seam, err := os.ReadFile(filepath.Join(
		"..", "..", "content", "blueprints", "rag-support", "app", "none", "infra", "provider.py"))
	if err != nil {
		t.Skip("blueprint not present in this tree")
	}
	for _, p := range providers {
		want := `"` + p.ID + `": "` + p.Key + `"`
		if !strings.Contains(string(seam), want) {
			t.Errorf("infra/provider.py does not map %s; expected %s", p.ID, want)
		}
	}
}

// setModelFields has to find `id` inside the model block and not the agent's own id, which appears
// earlier in the file. Getting that wrong renames the agent and leaves the model untouched — both
// halves silently wrong, and the manifest still validates.
func TestSetModelFieldsRewritesOnlyTheModelBlock(t *testing.T) {
	manifest := `agent:
  id: support-triage
  name: Support triage
  model:
    provider: anthropic
    id: claude-sonnet-4-5-20250929
    pinned: true
    params:
      temperature: 0.2
  tools:
    - id: lookup_order
`
	lines := strings.Split(manifest, "\n")
	if !setModelFields(lines, "openai", "gpt-5.6-terra") {
		t.Fatal("setModelFields found no model block")
	}
	got := strings.Join(lines, "\n")

	for _, want := range []string{
		"  id: support-triage",   // the agent's id, untouched
		"    provider: openai",   //
		"    id: gpt-5.6-terra",  //
		"    - id: lookup_order", // a tool id, untouched
		"      temperature: 0.2", // params, untouched
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
}

func TestSetProviderSDKReplacesTheMarkedLineOnlyOnce(t *testing.T) {
	req := []string{
		"# a comment mentioning anthropic",
		"anthropic==0.120.2  # agentarch:provider-sdk",
		"fastapi==0.141.1",
	}
	if !setProviderSDK(req, "openai==2.51.0") {
		t.Fatal("no marked line found")
	}
	if req[1] != "openai==2.51.0  "+providerSDKMarker {
		t.Errorf("rewrote to %q", req[1])
	}
	// Applying it again must be safe: the marker survives, and the line does not accumulate pins.
	if !setProviderSDK(req, "google-genai==2.16.0") {
		t.Fatal("the marker did not survive the first rewrite")
	}
	if req[1] != "google-genai==2.16.0  "+providerSDKMarker {
		t.Errorf("second rewrite produced %q", req[1])
	}
	if req[0] != "# a comment mentioning anthropic" || req[2] != "fastapi==0.141.1" {
		t.Error("a line without the marker was rewritten")
	}
}

// The interview has to accept the same values --provider does, by number and by id.
func TestProviderQuestionAcceptsNumbersAndIDs(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   string
		quit   bool
	}{
		{"\n", "anthropic", false}, // Enter takes the default
		{"1\n", "anthropic", false},
		{"2\n", "openai", false},
		{"3\n", "google", false},
		{"google\n", "google", false},
		{"OpenAI\n", "openai", false},
		{"q\n", "", true},
		{"nonsense\n1\n", "anthropic", false}, // a wrong answer is re-asked, not fatal
	} {
		withAnswers(t, tc.answer, func() {
			var got providerChoice
			var quit bool
			capture(t, func() int {
				got, quit = askProvider()
				return exitOK
			})
			if quit != tc.quit {
				t.Errorf("answer %q: quit = %v, want %v", tc.answer, quit, tc.quit)
			}
			if !tc.quit && got.ID != tc.want {
				t.Errorf("answer %q: chose %q, want %q", tc.answer, got.ID, tc.want)
			}
		})
	}
}
