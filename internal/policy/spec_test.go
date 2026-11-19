package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The specification is normative and the implementation is not. These tests fail when the two
// drift — a spec that describes something the code does not do is worse than no spec, because it
// invites a second implementation to be built against a fiction.

func specDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(moduleRoot(t), "spec")
}

func TestEveryNormativeDocumentExists(t *testing.T) {
	want := []string{
		"00-index.md", "01-manifest.md", "02-control-and-pack.md", "03-resolution.md",
		"04-expression-language.md", "05-shim-rendering.md", "06-exit-codes.md",
		"07-conformance-levels.md", "08-versioning.md",
	}
	for _, name := range want {
		p := filepath.Join(specDir(t), "normative", name)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("missing normative document: %s", name)
			continue
		}
		// A stub is worse than an absence: it makes the index look complete.
		if info.Size() < 500 {
			t.Errorf("%s is %d bytes — a stub reads as a promise the spec does not keep",
				name, info.Size())
		}
	}
}

// Every schema the manifest spec names must exist, or an implementation reading the spec is
// pointed at nothing.
func TestEverySchemaTheSpecNamesExists(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(specDir(t), "normative", "01-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"agent.manifest.schema.json", "tool.spec.schema.json", "mcp.allowlist.schema.json",
		"waivers.schema.json", "baseline.schema.json", "agentarch.config.schema.json",
	} {
		if !strings.Contains(string(raw), name) {
			t.Errorf("01-manifest.md does not name %s", name)
		}
		if _, err := os.Stat(filepath.Join(specDir(t), "schemas", name)); err != nil {
			t.Errorf("spec names %s but it does not exist", name)
		}
	}
}

// The exit codes in the spec are the ones the CLI uses. A divergence here is the kind that only
// surfaces in someone else's CI.
func TestExitCodesMatchTheSpec(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(specDir(t), "normative", "06-exit-codes.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	for _, row := range []struct {
		code int
		text string
	}{
		{0, "success"},
		{2, "structural validation failed"},
		{3, "generated files are out of date"},
		{4, "a blocker-severity control failed"},
		{5, "a waiver is invalid or has expired"},
		{6, "a revalidation trigger fired"},
	} {
		if !strings.Contains(body, row.text) {
			t.Errorf("exit code %d: the spec no longer describes %q", row.code, row.text)
		}
	}
}

// The budget in the spec is the budget the renderer enforces. If these drift, an implementation
// built from the spec produces files the reference implementation rejects.
func TestRenderingBudgetsMatchTheSpec(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(specDir(t), "normative", "05-shim-rendering.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, n := range []string{"12288", "8192", "6144"} {
		if !strings.Contains(body, n) {
			t.Errorf("05-shim-rendering.md no longer states the %s budget", n)
		}
	}
}

func TestConformanceSuiteIsDocumented(t *testing.T) {
	p := filepath.Join(specDir(t), "conformance", "README.md")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("the conformance suite has no README, so nobody knows how to run it: %v", err)
	}
	// The point that is easiest to lose and most important to keep.
	if !strings.Contains(string(raw), "reject") {
		t.Error("the README should say that rejection, not sandboxing, is the requirement")
	}

	// Every suite directory has to be documented. A fixture nobody knows to run is a fixture a
	// second implementation does not run, which is the same as not having it.
	dirs, err := os.ReadDir(filepath.Join(specDir(t), "conformance"))
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		found++
		if !strings.Contains(string(raw), d.Name()+"/") {
			t.Errorf("spec/conformance/%s/ exists and the README does not mention it", d.Name())
		}
	}
	if found == 0 {
		t.Fatal("the conformance suite has no fixture directories")
	}
}

// spec/normative/07-conformance-levels.md Part 2 lists five requirements for a spec/1.0
// implementation. For a long time only the first had fixtures, and the argument that a second
// implementation is practical — which GOVERNANCE.md §5 offers as the structural answer to having
// one maintainer — rested on a suite covering a fifth of the contract.
func TestConformanceSuiteCoversTheImplementationRequirements(t *testing.T) {
	want := map[string]string{
		"expr":       "the expression language and what must be rejected",
		"exit-codes": "the exit codes and their precedence",
		"resolution": "pack resolution: union, highest severity, binding-law floor",
		"budgets":    "the rendering budgets, as errors rather than truncations",
	}
	for dir, covers := range want {
		p := filepath.Join(specDir(t), "conformance", dir, "cases.yaml")
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("no fixtures for %s (%s). Without them an implementation has to read "+
				"the Go, and the specification is documentation about whatever that code "+
				"happens to do.", dir, covers)
			continue
		}
		// A stub is worse than an absence: it makes the suite look complete.
		if info.Size() < 512 {
			t.Errorf("spec/conformance/%s/cases.yaml is %d bytes — too small to cover %s",
				dir, info.Size(), covers)
		}
	}
}

// controlTypePattern pulls the control-id regex out of a schema, wherever in the document it
// sits. Three schemas carry it and they are meant to be identical.
func controlTypePattern(t *testing.T, schema string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(specDir(t), "schemas", schema))
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	var found string
	var walk func(any)
	walk = func(n any) {
		m, ok := n.(map[string]any)
		if !ok {
			if arr, ok := n.([]any); ok {
				for _, v := range arr {
					walk(v)
				}
			}
			return
		}
		if p, ok := m["pattern"].(string); ok && strings.HasPrefix(p, `^control\.ai\.`) {
			if found == "" {
				found = p
			} else if found != p {
				t.Errorf("%s carries two different control-id patterns", schema)
			}
		}
		for _, v := range m {
			walk(v)
		}
	}
	walk(doc)

	if found == "" {
		t.Fatalf("%s declares no control-id pattern", schema)
	}
	return found
}

// The control type vocabulary is closed, and it is written down in three schemas. When `api` was
// added in content 1.1 it reached control.schema.json and not agent.manifest.schema.json, so a
// manifest declaring a guardrail against control.ai.api.* was rejected by the schema written to
// accept it — and nothing noticed, because no shipped manifest happened to do that.
//
// Meanwhile waivers.schema.json matched `[a-z]+`, which accepts a waiver against a control type
// that cannot exist: it reports as a waived exception and suppresses nothing.
func TestEverySchemaAgreesOnTheControlVocabulary(t *testing.T) {
	schemas := []string{
		"control.schema.json",
		"agent.manifest.schema.json",
		"waivers.schema.json",
	}

	want := controlTypePattern(t, schemas[0])
	for _, s := range schemas[1:] {
		if got := controlTypePattern(t, s); got != want {
			t.Errorf("%s control-id pattern is\n  %s\nbut %s has\n  %s\n"+
				"One closed vocabulary, written once — a type accepted in one schema and "+
				"refused in another is a manifest nobody can write.", s, got, schemas[0], want)
		}
	}

	// The pattern must actually enumerate the types. A wildcard passes the comparison above
	// while enforcing nothing.
	for _, typ := range []string{"agent", "genai", "rag", "tool", "mcp", "api", "privacy",
		"eval", "obs", "supply", "lifecycle"} {
		if !strings.Contains(want, typ) {
			t.Errorf("control type %q is missing from the schema vocabulary", typ)
		}
	}
	if strings.Contains(want, `[a-z]+\.`) {
		t.Error("the control-id pattern accepts any type; the vocabulary is meant to be closed")
	}
}
