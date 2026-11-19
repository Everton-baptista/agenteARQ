package render

import (
	"strings"
	"testing"
	"testing/fstest"
)

func testFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func TestBuildCoreConcatenatesInFilenameOrder(t *testing.T) {
	fsys := testFS(map[string]string{
		"core/en/30-routing.md":    "third",
		"core/en/00-identity.md":   "first",
		"core/en/10-invariants.md": "second",
		"core/en/notes.txt":        "ignored",
	})
	core, err := BuildCore(fsys, "en")
	if err != nil {
		t.Fatal(err)
	}
	if want := "first\nsecond\nthird"; core.Text != want {
		t.Fatalf("core text = %q, want %q", core.Text, want)
	}
	if len(core.Files) != 3 {
		t.Fatalf("expected 3 core files, got %v", core.Files)
	}
}

func TestBuildCoreDigestChangesWithContent(t *testing.T) {
	a, _ := BuildCore(testFS(map[string]string{"core/en/00.md": "one"}), "en")
	b, _ := BuildCore(testFS(map[string]string{"core/en/00.md": "two"}), "en")
	if a.SHA256 == b.SHA256 {
		t.Fatal("digest must change when the core changes, or drift detection is blind")
	}
}

// The budget is the mechanism that keeps the core scarce. If it ever degrades to a warning,
// the core grows until assistants stop reading it — so this test asserts a hard error.
func TestRenderRefusesToExceedBudget(t *testing.T) {
	core := Core{Text: strings.Repeat("x", 9000), SHA256: strings.Repeat("a", 64), Lang: "en"}
	tgt := Target{Name: "copilot", Path: ".github/copilot-instructions.md", Budget: 6144}

	if _, err := Render(tgt, core, "1.0.0", ""); err == nil {
		t.Fatal("expected an error when the rendered file exceeds its budget")
	} else if !strings.Contains(err.Error(), "budget") {
		t.Fatalf("error should name the budget, got: %v", err)
	}
}

func TestRenderPreservesCustomRegions(t *testing.T) {
	core := Core{Text: "core rules", SHA256: strings.Repeat("a", 64), Lang: "en"}
	tgt := Target{Name: "claude", Path: "CLAUDE.md", Budget: 12288}

	existing := "<!-- agentarch:custom:start -->\nour own note\n<!-- agentarch:custom:end -->"
	out, err := Render(tgt, core, "1.0.0", existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "our own note") {
		t.Fatal("custom region was dropped; without a working escape hatch teams stop running sync")
	}

	// Rendering the output again must be a no-op, or every sync reports drift forever.
	again, err := Render(tgt, core, "1.0.0", out)
	if err != nil {
		t.Fatal(err)
	}
	if again != out {
		t.Fatal("render is not idempotent over its own output")
	}
}

func TestRenderEmbedsDigestSoDriftIsDetectable(t *testing.T) {
	core := Core{Text: "rules", SHA256: strings.Repeat("b", 64), Lang: "en"}
	out, err := Render(Target{Name: "claude", Path: "CLAUDE.md", Budget: 4096}, core, "1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := CoreSHAOf(out); got != core.SHA256 {
		t.Fatalf("digest not recoverable from output: got %q", got)
	}
	if CoreSHAOf("a hand-written file") != "" {
		t.Fatal("a file with no header must report no digest, so it is not mistaken for stale output")
	}
}

func TestEveryTargetHasABudgetAndDistinctPath(t *testing.T) {
	paths := map[string]string{}
	for _, tg := range Targets {
		if tg.Budget <= 0 {
			t.Errorf("target %s has no budget", tg.Name)
		}
		if prev, dup := paths[tg.Path]; dup {
			t.Errorf("targets %s and %s both write %s", prev, tg.Name, tg.Path)
		}
		paths[tg.Path] = tg.Name
	}
}
