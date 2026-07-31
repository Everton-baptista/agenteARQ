package main

// Which model provider the project is built against.
//
// Every blueprint ships a seam — app/infra/provider.py, with one module per provider under
// app/infra/providers/ — so the choice reaches three places and no further: `model.provider` and
// `model.id` in the manifest, and the pinned SDK line in app/requirements.txt. The agent core does
// not change, which is the point of the seam and the reason this question is cheap to ask.
//
// It is asked after "what are you building", because only then is it known whether a model is
// involved at all, and before "where are the users", because the jurisdiction question is the last
// one and it should stay last — it is the one people pause on.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// providerChoice is one row of the table: what the interview offers and what gets written.
type providerChoice struct {
	ID      string // model.provider in the manifest, and what --provider takes
	Label   string // shown in the menu
	ModelID string // model.id — an exact id, never an alias (invariant 7)
	SDK     string // the pinned line for app/requirements.txt
	Key     string // the credential name, matching CREDENTIAL_NAMES in the seam
}

// providers is the table, and it is the only part of agentarch that goes stale on its own.
//
// Invariant 7 says pin the model, because a floating alias changes behaviour under you between two
// deploys of the same commit. Worth stating what "pinned" means per provider, because it is not the
// same thing in all three: OpenAI and Google ids name a specific model, and Anthropic's
// current-generation ids carry no date suffix at all — appending one is rejected — so the exact
// string below *is* the pin.
//
// The Anthropic row records what the blueprints already ship rather than the newest model. Choosing
// `anthropic` therefore changes nothing, which is deliberate: the default path must not be a silent
// model migration. Moving the blueprints to a newer Anthropic model is its own change, touching the
// manifests, the examples and the conformance cases together.
//
// TestProviderTableIsReviewed turns providersReviewed into a deadline. A pinned id that has been
// discontinued is worse than a floating alias: it looks like a decision and is an oversight.
var providers = []providerChoice{
	{
		ID:      "anthropic",
		Label:   "Anthropic (Claude)",
		ModelID: "claude-sonnet-4-5-20250929",
		SDK:     "anthropic==0.120.2",
		Key:     "ANTHROPIC_API_KEY",
	},
	{
		ID:      "openai",
		Label:   "OpenAI (GPT)",
		ModelID: "gpt-5.6-terra",
		SDK:     "openai==2.51.0",
		Key:     "OPENAI_API_KEY",
	},
	{
		ID:      "google",
		Label:   "Google (Gemini)",
		ModelID: "gemini-3.6-flash",
		SDK:     "google-genai==2.16.0",
		Key:     "GOOGLE_API_KEY",
	},
}

// providersReviewed is the day the ids and pins above were last checked against each provider's own
// documentation. Bump it when you check, not when you edit something nearby.
var providersReviewed = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)

// providerReviewInterval is how long the table is trusted without being looked at again.
//
// Six months is short enough that a discontinued model is caught before somebody's project stops
// working, and long enough not to be noise. The check is a test rather than a runtime warning: a
// warning printed to a user is a maintenance task delegated to the wrong person.
const providerReviewInterval = 183 * 24 * time.Hour

func findProvider(id string) (providerChoice, bool) {
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}
	return providerChoice{}, false
}

func providerIDs() []string {
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p.ID
	}
	return out
}

// askProvider is the question. The default is the first row, which is what the blueprints ship.
func askProvider() (providerChoice, bool) {
	fmt.Printf("\n%s\n\n", t("provider.question"))
	for i, p := range providers {
		fmt.Printf("  %d. %-22s %s\n", i+1, p.Label, p.ModelID)
	}
	fmt.Printf("\n%s\n\n", t("provider.explain"))

	for attempt := 0; attempt < 3; attempt++ {
		in := strings.ToLower(ask(tf("provider.prompt", len(providers))))
		switch in {
		case "":
			return providers[0], false
		case "q", "quit", "sair":
			fmt.Println(t("plan.nothing"))
			return providerChoice{}, true
		}
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(providers) {
			return providers[n-1], false
		}
		// By id too — somebody who typed `openai` meant it.
		if p, ok := findProvider(in); ok {
			return p, false
		}
		fmt.Fprintln(os.Stderr, t("common.notanoption"))
	}
	fmt.Fprintln(os.Stderr, t("common.givingup"))
	return providers[0], true
}

// ------------------------------------------------------- writing the answer in

// modelBlockRe finds the `model:` mapping the manifest declares the provider under.
//
// Scoped rather than by key alone, because `id` appears earlier in the file as the agent's own id
// and `setField` matches the first occurrence — rewriting that one would rename the agent and leave
// the model untouched, which is the worst of both.
var modelBlockRe = regexp.MustCompile(`^(\s*)model:\s*$`)

// setModelFields rewrites provider and id inside the manifest's model block.
func setModelFields(lines []string, provider, modelID string) bool {
	for i, line := range lines {
		m := modelBlockRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent := len(m[1])
		wrote := 0
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				continue
			}
			// Dedent to the model key's level or above ends the block.
			if len(next)-len(strings.TrimLeft(next, " ")) <= indent {
				break
			}
			f := fieldRe.FindStringSubmatch(next)
			if f == nil {
				continue
			}
			switch f[2] {
			case "provider":
				lines[j] = f[1] + "provider: " + provider
				wrote++
			case "id":
				lines[j] = f[1] + "id: " + modelID
				wrote++
			}
			if wrote == 2 {
				return true
			}
		}
		return wrote > 0
	}
	return false
}

// providerSDKMarker is what the blueprint's requirements.txt carries on the line to replace. A
// marker rather than a match on the package name: after the first switch the line no longer says
// `anthropic`, and a rewrite that only knows the old name can be applied exactly once.
const providerSDKMarker = "# agentarch:provider-sdk"

func setProviderSDK(lines []string, pin string) bool {
	for i, line := range lines {
		if !strings.Contains(line, providerSDKMarker) {
			continue
		}
		lines[i] = pin + "  " + providerSDKMarker
		return true
	}
	return false
}

// applyProvider writes the chosen provider into everything that has to agree about it.
//
// Returns the number of files changed, so the caller can say so rather than claim it silently. A
// zero for a non-default provider is a bug worth surfacing: it means the manifest still names one
// provider while requirements.txt pins another SDK, and the failure would arrive much later as an
// import error at the first model call.
func applyProvider(root string, p providerChoice) (int, error) {
	changed := 0

	dir := filepath.Join(root, "agentarch", "project", "agents")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "agent.yaml")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(raw), "\n")
		if !setModelFields(lines, p.ID, p.ModelID) {
			continue
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return changed, err
		}
		changed++
	}

	req := filepath.Join(root, "app", "requirements.txt")
	raw, err := os.ReadFile(req)
	if err != nil {
		return changed, nil // a blueprint with no Python app is not an error
	}
	lines := strings.Split(string(raw), "\n")
	if setProviderSDK(lines, p.SDK) {
		if err := os.WriteFile(req, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
