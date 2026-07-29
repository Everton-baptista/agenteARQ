package blueprint_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/blueprint"
)

func content(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(agentarch.Content, "content")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func load(t *testing.T) []blueprint.Blueprint {
	t.Helper()
	bps, err := blueprint.Load(content(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(bps) == 0 {
		t.Fatal("no blueprints shipped")
	}
	return bps
}

// The catalogue is browsed by need. Someone arriving does not know the ids — they know what
// they are trying to build.
func TestEveryBlueprintStatesItsNeedAndWhatItShips(t *testing.T) {
	for _, b := range load(t) {
		if len(b.Meta.Need) < 20 {
			t.Errorf("%s: need is too short to recognise yourself in", b.Meta.ID)
		}
		if b.Meta.Title == "" {
			t.Errorf("%s: no title", b.Meta.ID)
		}
		if len(b.Meta.Frameworks) == 0 {
			t.Errorf("%s: claims no runnable code at all", b.Meta.ID)
		}
		if len(b.Meta.Demonstrates) < 3 {
			t.Errorf("%s: a starting point should show at least three things", b.Meta.ID)
		}
	}
}

// Claiming a framework it does not ship would be the same dishonesty the standard exists to
// prevent, and the reader only finds out after installing.
func TestFrameworkClaimsAreBackedByFiles(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		for _, fw := range b.Meta.Frameworks {
			dir := b.Root + "/app/" + fw
			entries, err := fs.ReadDir(src, dir)
			if err != nil || len(entries) == 0 {
				t.Errorf("%s claims %q but ships no code at %s", b.Meta.ID, fw, dir)
			}
		}
	}
}

// A starting point that does not run is documentation with extra steps.
func TestEveryBlueprintShipsAnEntryPointAndInstructions(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		for _, fw := range b.Meta.Frameworks {
			for _, want := range []string{"agent.py", "README.md", "requirements.txt"} {
				if _, err := fs.Stat(src, b.Root+"/app/"+fw+"/"+want); err != nil {
					t.Errorf("%s/%s is missing %s", b.Meta.ID, fw, want)
				}
			}
		}
	}
}

// The manifest is the contract. A blueprint that ships no manifest ships no contract.
func TestEveryBlueprintShipsAManifest(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		found := false
		_ = fs.WalkDir(src, b.Root+"/agentarch/project/agents", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Name() == "agent.yaml" {
				found = true
			}
			return nil
		})
		if !found {
			t.Errorf("%s ships no agent manifest", b.Meta.ID)
		}
	}
}

func TestAskingForAFrameworkItDoesNotShipFailsClearly(t *testing.T) {
	bps := load(t)
	b := bps[0]

	_, err := blueprint.Prepare(content(t), b, t.TempDir(), "langgraph")
	if err == nil {
		t.Fatal("expected an error for a framework the blueprint does not ship")
	}
	// The message has to say what it does ship, or the reader is left guessing.
	if !strings.Contains(err.Error(), b.Meta.Frameworks[0]) {
		t.Errorf("the error should list what is available, got: %v", err)
	}
}

// Writing first and reporting afterwards is how a scaffolding tool loses someone's work.
func TestPrepareTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	b := load(t)[0]

	plan, err := blueprint.Prepare(content(t), b, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) == 0 {
		t.Fatal("plan is empty")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("Prepare wrote %d entries; it must only plan", len(entries))
	}
}

func TestConflictsAreReportedBeforeAnythingIsWritten(t *testing.T) {
	dir := t.TempDir()
	b := load(t)[0]

	plan, err := blueprint.Prepare(content(t), b, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	// Put something where the blueprint would write.
	victim := filepath.Join(dir, filepath.FromSlash(plan.Files[0]))
	if err := os.MkdirAll(filepath.Dir(victim), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("someone's work"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan2, err := blueprint.Prepare(content(t), b, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan2.Conflicts) != 1 {
		t.Fatalf("expected the existing file to be reported, got %v", plan2.Conflicts)
	}

	if got, _ := os.ReadFile(victim); string(got) != "someone's work" {
		t.Fatal("Prepare overwrote a file it was supposed to report")
	}
}

// Only the chosen framework's code is written, and it lands at app/ rather than
// app/<framework>/ — the project should not carry a directory named after a choice it made.
func TestApplyWritesOnlyTheChosenFramework(t *testing.T) {
	dir := t.TempDir()
	b := load(t)[0]
	fw := b.Meta.Frameworks[0]

	plan, err := blueprint.Prepare(content(t), b, dir, fw)
	if err != nil {
		t.Fatal(err)
	}
	if err := blueprint.Apply(content(t), b, dir, plan); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "app", "agent.py")); err != nil {
		t.Error("app/agent.py was not written")
	}
	if _, err := os.Stat(filepath.Join(dir, "app", fw)); err == nil {
		t.Error("the framework directory leaked into the installed project")
	}
	if _, err := os.Stat(filepath.Join(dir, "blueprint.yaml")); err == nil {
		t.Error("catalogue metadata was written into the project")
	}
}

func TestFindByID(t *testing.T) {
	bps := load(t)
	if _, ok := blueprint.Find(bps, bps[0].Meta.ID); !ok {
		t.Error("a shipped blueprint was not findable by its own id")
	}
	if _, ok := blueprint.Find(bps, "no-such-blueprint"); ok {
		t.Error("found a blueprint that does not exist")
	}
}
