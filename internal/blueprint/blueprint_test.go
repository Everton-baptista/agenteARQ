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
//
// Two entry points, and both are required. The service is what reaches production; the CLI is what
// proves the agent core does not need a web server, which is the claim the whole layout rests on.
func TestEveryBlueprintShipsAnEntryPointAndInstructions(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		for _, fw := range b.Meta.Frameworks {
			for _, want := range []string{
				"api/main.py", "api/routes.py", "api/deps.py", "api/schemas.py",
				"agent/runner.py", "agent/guardrails.py", "agent/principal.py",
				"cli.py", "README.md", "requirements.txt",
			} {
				if _, err := fs.Stat(src, b.Root+"/app/"+fw+"/"+want); err != nil {
					t.Errorf("%s/%s is missing %s", b.Meta.ID, fw, want)
				}
			}
		}
	}
}

// Every blueprint has to make `.env` ignored, because control.ai.api.secrets_not_committed depends
// on it and a blueprint that ships a service without that is shipping the most common way a
// provider credential reaches a public repository.
func TestEveryBlueprintIgnoresTheEnvFileAndDocumentsIt(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		ignore, err := fs.ReadFile(src, b.Root+"/.gitignore")
		if err != nil {
			t.Errorf("%s ships no .gitignore", b.Meta.ID)
			continue
		}
		if !strings.Contains(string(ignore), ".env") {
			t.Errorf("%s does not ignore .env", b.Meta.ID)
		}
		example, err := fs.ReadFile(src, b.Root+"/.env.example")
		if err != nil {
			t.Errorf("%s ships no .env.example", b.Meta.ID)
			continue
		}
		// Names, never values. A committed example with a value in it is a committed secret with
		// a reassuring filename.
		for _, line := range strings.Split(string(example), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, _ := strings.Cut(line, "=")
			if strings.Contains(key, "KEY") || strings.Contains(key, "SECRET") ||
				strings.Contains(key, "TOKEN") || strings.Contains(key, "PASSWORD") {
				if strings.TrimSpace(value) != "" {
					t.Errorf("%s/.env.example assigns a value to %s", b.Meta.ID, key)
				}
			}
		}
	}
}

// No blueprint may ship an evaluation result that claims to have been measured.
//
// This is the test that should have existed first. The rag-support blueprint shipped groundedness
// 0.94, a jailbreak rate of 0.03 and sixty red team cases against datasets that did not exist —
// all invented — and `agentarch conformance` read them and reported L3 Proven for a project one
// minute old. The standard committed the conformance theatre it exists to prevent, in the first
// artifact a new user is handed.
//
// A blueprint hands someone a starting point. It may hand them thresholds, because deciding what
// you are committing to is real work it can do for you. It may never hand them evidence.
func TestNoBlueprintShipsFabricatedEvidence(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		_ = fs.WalkDir(src, b.Root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.Contains(p, "/evals/results/") {
				return nil
			}
			body, rerr := fs.ReadFile(src, p)
			if rerr != nil {
				return nil
			}
			// Matched at the start of a line, not as a substring. These files explain themselves
			// at length, and the explanation contains the phrase "sets status: measured" — a
			// substring match failed on the prose that exists to prevent the very thing it
			// describes.
			status := ""
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "status:") {
					status = strings.TrimSpace(strings.TrimPrefix(line, "status:"))
					break
				}
			}
			if status != "not_run" {
				t.Errorf("%s declares status %q — a blueprint may ship thresholds, never evidence", p, status)
			}
			// The specific shapes of the original lie, named so the test explains itself when it
			// fails rather than sending the reader to git history. Also anchored, for the same
			// reason.
			for i, line := range strings.Split(string(body), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") {
					continue
				}
				for _, tell := range []string{"executed: true", "passed: true", "passed: false"} {
					if strings.Contains(trimmed, tell) {
						t.Errorf("%s:%d has %q — an unmeasured result carries nulls", p, i+1, tell)
					}
				}
			}
			return nil
		})
	}
}

// The rule the layout exists for, checked from Go so it runs in CI with no Python installed.
//
// It reads import lines textually and will not catch a violation smuggled through a string. It
// catches the one people actually make — reaching into the transport layer from the agent to read
// a header — which is the drift that ends with an agent that only runs inside a web server.
func TestTheAgentCoreNeverImportsTheTransport(t *testing.T) {
	src := content(t)
	for _, b := range load(t) {
		for _, fw := range b.Meta.Frameworks {
			dir := b.Root + "/app/" + fw + "/agent"
			_ = fs.WalkDir(src, dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(p, ".py") {
					return nil
				}
				body, rerr := fs.ReadFile(src, p)
				if rerr != nil {
					return nil
				}
				for i, line := range strings.Split(string(body), "\n") {
					s := strings.TrimSpace(line)
					if !strings.HasPrefix(s, "import ") && !strings.HasPrefix(s, "from ") {
						continue
					}
					for _, forbidden := range []string{"fastapi", "starlette", "..api", "app.api"} {
						if strings.Contains(s, forbidden) {
							t.Errorf("%s:%d imports %s — agent/ must not depend on the transport",
								p, i+1, forbidden)
						}
					}
				}
				return nil
			})
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

	// The collapse itself: app/<framework>/api/main.py lands at app/api/main.py, nested directories
	// intact. An earlier version of this test looked for a single app/agent.py, which stopped saying
	// anything the moment the blueprints became services with a directory tree.
	for _, want := range []string{
		filepath.Join("app", "api", "main.py"),
		filepath.Join("app", "agent", "runner.py"),
		filepath.Join("app", "cli.py"),
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("%s was not written", want)
		}
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
