package main

// Smoke tests for init, check and upgrade wiring: flag validation, what reaches the config
// file, and which version `upgrade` announces.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/render"
)

func TestInitRejectsNonISOJurisdiction(t *testing.T) {
	_, stderr, code := capture(t, func() int {
		return cmdInit([]string{"--root", t.TempDir(), "--jurisdictions", "brasil"})
	})
	if code != exitUsage {
		t.Fatalf("init --jurisdictions brasil exited %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "ISO 3166-1 alpha-2") {
		t.Errorf("stderr should explain the expected format, got:\n%s", stderr)
	}
}

func TestInitNormalizesJurisdictions(t *testing.T) {
	root := t.TempDir()

	_, stderr, code := capture(t, func() int {
		return cmdInit([]string{"--root", root, "--jurisdictions", "br,eu"})
	})
	if code != exitOK {
		t.Fatalf("init --jurisdictions br,eu exited %d, want 0\nstderr:\n%s", code, stderr)
	}

	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		t.Fatalf("reading agentarch.yaml: %v", err)
	}
	if !strings.Contains(string(raw), `jurisdictions: ["BR", "EU"]`) {
		t.Errorf("agentarch.yaml should record the codes uppercased, got:\n%s", raw)
	}
}

func TestCheckUpdateBaselineWithoutBaseline(t *testing.T) {
	root := t.TempDir()

	if _, stderr, code := capture(t, func() int { return cmdInit([]string{"--root", root}) }); code != exitOK {
		t.Fatalf("init exited %d, want 0\nstderr:\n%s", code, stderr)
	}
	// One agent must exist, otherwise check exits earlier with "no agents matched" and the
	// missing-baseline branch is never reached.
	if _, stderr, code := capture(t, func() int {
		return cmdNewAgent([]string{"--root", root, "smoke-agent"})
	}); code != exitOK {
		t.Fatalf("new agent exited %d, want 0\nstderr:\n%s", code, stderr)
	}

	_, stderr, code := capture(t, func() int {
		return cmdCheck([]string{"--root", root, "--update-baseline"})
	})
	if code != exitUsage {
		t.Fatalf("check --update-baseline without a baseline exited %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "no baseline recorded yet") {
		t.Errorf("stderr should say there is no baseline to update, got:\n%s", stderr)
	}
}

func TestUpgradeDryRunAnnouncesContentVersion(t *testing.T) {
	root := t.TempDir()

	if _, stderr, code := capture(t, func() int { return cmdInit([]string{"--root", root}) }); code != exitOK {
		t.Fatalf("init exited %d, want 0\nstderr:\n%s", code, stderr)
	}

	stdout, _, code := capture(t, func() int {
		return cmdUpgrade([]string{"--root", root, "--dry-run"})
	})
	if code != exitOK {
		t.Fatalf("upgrade --dry-run exited %d, want 0", code)
	}
	if !strings.Contains(stdout, "would replace") {
		t.Errorf("dry-run should report what it would replace, got:\n%s", stdout)
	}

	// The announced version is the content this binary carries, never the CLI's own version.
	embedded, err := fs.Sub(agentarch.Content, "content")
	if err != nil {
		t.Fatalf("fs.Sub on the embedded payload: %v", err)
	}
	want := render.ContentVersion(embedded)
	if want == version {
		t.Fatalf("test premise broken: content version %q equals the CLI version", want)
	}
	if !strings.Contains(stdout, "content "+want) {
		t.Errorf("dry-run should announce content %s, got:\n%s", want, stdout)
	}
}

// shippedTrees are the directories in this repository that carry generated instruction files
// under version control. Everything else renders them at install time.
func shippedTrees(t *testing.T) []string {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out := []string{repo}

	entries, err := os.ReadDir(filepath.Join(repo, "examples"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "99-") {
			continue
		}
		out = append(out, filepath.Join(repo, "examples", e.Name()))
	}
	return out
}

// committedTargets returns the names of the targets whose files are actually checked in under
// root, so the assertion covers what the repository ships rather than what it could ship.
func committedTargets(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	for _, tg := range render.Targets {
		if tg.Name == "mcp_json" || tg.Name == "skills" || tg.Name == "openapi" {
			continue // derived from an artifact, not rendered from the core
		}
		b, err := os.ReadFile(filepath.Join(root, tg.Path))
		if err != nil {
			continue
		}
		if render.CoreSHAOf(string(b)) != "" {
			names = append(names, tg.Name)
		}
	}
	return names
}

// A core edit regenerates the repository's own shims and leaves every other committed tree
// behind. That has happened twice, and both times the only thing that noticed was a CI step far
// enough down the job that an earlier failure hid it — so the reference example sat at
// conformance "none" while claiming to demonstrate L3.
//
// Checking it here means the commit that causes it fails on the machine that made it.
func TestNoCommittedShimIsStale(t *testing.T) {
	for _, root := range shippedTrees(t) {
		targets := committedTargets(t, root)
		if len(targets) == 0 {
			continue
		}
		t.Run(filepath.Base(root), func(t *testing.T) {
			args := []string{"--check", "--root", root, "--targets", strings.Join(targets, ",")}
			stdout, stderr, code := capture(t, func() int { return cmdSync(args) })
			if code != exitOK {
				t.Errorf("%s has stale generated files (exit %d). Run:\n"+
					"    agentarch sync --root %s\n\n%s%s",
					root, code, root, stdout, stderr)
			}
		})
	}
}

// generatedFile reports whether a name is an assistant instruction file the spec describes as
// output. Those are rendered into an adopter's project and are not documents of this repository.
func generatedFile(name string) bool {
	for _, t := range render.Targets {
		if filepath.Base(t.Path) == name {
			return true
		}
	}
	return false
}

// A policy document the normative spec cites has to exist. TRADEMARK.md was cited twice — from
// the index, and from the end of the conformance definition, as the thing that governs the
// compliance claim — and was never written. That is the one question a second implementer has to
// answer before publishing anything, and the spec answered it with a dangling pointer.
func TestEveryPolicyDocumentTheSpecCitesExists(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, "spec", "normative")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	cited := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range citedDoc.FindAllStringSubmatch(string(raw), -1) {
			if generatedFile(m[1]) {
				continue
			}
			cited[m[1]] = append(cited[m[1]], e.Name())
		}
	}

	if len(cited) == 0 {
		t.Fatal("no citations found; the extraction is broken, not the spec")
	}

	for doc, from := range cited {
		if _, err := os.Stat(filepath.Join(repo, doc)); err != nil {
			t.Errorf("%s is cited by %s and does not exist. A spec that points at a missing "+
				"document answers the reader's question with authority and nothing else.",
				doc, strings.Join(from, ", "))
		}
	}
}

// Repository policy documents are SHOUTED and carry .md: TRADEMARK.md, CONTRIBUTING.md,
// GOVERNANCE.md. Lowercase paths in the spec are spec-relative and covered elsewhere.
var citedDoc = regexp.MustCompile("`([A-Z][A-Z-]+\\.md)`")

// spec/normative/08-versioning.md: "An implementation SHOULD print all three versions on request;
// version output that names only the binary leaves the reader unable to reproduce a result." The
// content line was the one missing, and it is the one that changes the answer: a project pinned
// to an older content release is judged by that release.
func TestVersionPrintsAllThreeLines(t *testing.T) {
	stdout, _, code := capture(t, func() int { return cmdVersion(nil) })
	if code != exitOK {
		t.Fatalf("version exited %d", code)
	}
	for _, want := range []string{"agentarch ", "spec/", "content/"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("version output has no %q line:\n%s", want, stdout)
		}
	}
	if n := len(strings.Split(strings.TrimSpace(stdout), "\n")); n != 3 {
		t.Errorf("version printed %d line(s), want 3:\n%s", n, stdout)
	}
}

// The matrix is a promise 08-versioning.md and GOVERNANCE.md both make. It was never published,
// so a reader with two different results and two different binaries had nothing to consult.
func TestCompatibilityMatrixIsPublished(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "Compatibility matrix") {
		t.Fatal("CHANGELOG.md publishes no compatibility matrix, which 08-versioning.md §46 " +
			"and GOVERNANCE.md §4 both promise per release")
	}
	for _, want := range []string{"Spec majors", "Content"} {
		if !strings.Contains(body, want) {
			t.Errorf("the matrix has no %q column; it has to name all three lines", want)
		}
	}
}
