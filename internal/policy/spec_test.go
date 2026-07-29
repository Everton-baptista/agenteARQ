package policy_test

import (
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
}
