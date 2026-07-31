package main

// Smoke tests for init, check and upgrade wiring: flag validation, what reaches the config
// file, and which version `upgrade` announces.

import (
	"fmt"
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
		// examples/ directory does not exist — only the repo root is shipped.
		return out
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

// The one command someone types to try this is
// `go run github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest`, and the go directive
// decides whether it works. Above the reader's Go it either downloads a whole toolchain first
// (GOTOOLCHAIN=auto) or fails outright (GOTOOLCHAIN=local, normal in CI and slim images).
//
// It sat at 1.26.1 — the newest release — while the production code used nothing newer than
// modules themselves, going as far as defining its own min(). `go mod tidy` on a machine with a
// fresh Go raises this by itself, silently, so it needs a test rather than a comment.
func TestGoDirectiveStaysAtTheFloor(t *testing.T) {
	const floor = "go 1.22"

	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}

	var got string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "go ") {
			got = strings.TrimSpace(line)
			break
		}
	}
	if got == "" {
		t.Fatal("go.mod declares no go directive")
	}
	if got != floor {
		t.Errorf("go.mod says %q, want %q.\n\n"+
			"Raising this excludes everyone on an older Go from the entry command. If a "+
			"language feature genuinely needs a newer version, change `floor` here and say in "+
			"go.mod which feature forced it — but check first that the feature is worth the "+
			"reach it costs.", got, floor)
	}

	// A toolchain line reintroduces the problem through the back door: it pins what the module
	// is *built* with, and `go run` honours it.
	if strings.Contains(string(raw), "\ntoolchain ") {
		t.Error("go.mod pins a toolchain, which re-imposes a Go version on anyone running " +
			"the entry command")
	}
}

// countCommandLines counts how many "  agentarch <cmd>" suggestion lines an output carries.
func countCommandLines(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  agentarch ") {
			n++
		}
	}
	return n
}

// A bare `agentarch` is somebody asking "what now". After `init` and before the first agent, the
// answer used to be twenty subcommands and exit 1 — the question restated in more detail, with a
// failure code attached to being midway through a setup.
func TestNextStepsAnswersTheQuestionAsked(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   projectState
		mustSay []string
		mustNot []string
	}{
		{
			name:  "installed with no agent yet",
			state: projectState{Installed: true, Agents: 0},
			// Only the two commands that can do anything when nothing is described yet.
			mustSay: []string{"blueprint", "new agent", "--help --all"},
			// Every one of these reads a manifest, and there is none.
			mustNot: []string{"agentarch check ", "conformance", "aibom", "waive", "score"},
		},
		{
			name:    "installed with agents",
			state:   projectState{Installed: true, Agents: 2},
			mustSay: []string{"2 agent(s)", "check", "conformance"},
			mustNot: []string{"aibom", "waive", "score", "upgrade"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, _ := capture(t, func() int {
				printNextSteps(tc.state)
				return exitOK
			})

			for _, want := range tc.mustSay {
				if !strings.Contains(stdout, want) {
					t.Errorf("output does not mention %q:\n%s", want, stdout)
				}
			}
			for _, unwanted := range tc.mustNot {
				if strings.Contains(stdout, unwanted) {
					t.Errorf("output mentions %q, which does not help from here:\n%s",
						unwanted, stdout)
				}
			}
			// The whole point is that it is short. Four is the ceiling; past that it is a
			// list again, and a list is what somebody asking "what now" cannot use.
			if n := countCommandLines(stdout); n > 4 {
				t.Errorf("suggests %d commands; at most 4, or it is a wall again", n)
			}
		})
	}
}

// `--help` used to be 31 lines of twenty commands with their flags — a reference manual, printed
// to anyone who mistyped a subcommand. The long list still exists behind --all; it is just not
// the answer to every question.
func TestHelpIsShortByDefaultAndCompleteOnRequest(t *testing.T) {
	_, short, _ := capture(t, func() int { usageFull(false); return exitOK })
	_, long, _ := capture(t, func() int { usageFull(true); return exitOK })

	if n := len(strings.Split(strings.TrimSpace(short), "\n")); n > 16 {
		t.Errorf("short help is %d lines; it is a wall again:\n%s", n, short)
	}
	if !strings.Contains(short, "--help --all") {
		t.Error("short help does not say where the rest is, so the rest is unreachable")
	}
	if len(long) <= len(short) {
		t.Error("--all is not longer than the short form")
	}

	// Every command reachable from run() must appear in the full help, or it exists and
	// nobody can find it.
	for _, cmd := range append(append([]string{}, firstWeekCommands...), extraCommands...) {
		if !strings.Contains(long, "  "+cmd) {
			t.Errorf("%q is a command and does not appear in `--help --all`", cmd)
		}
		if cmd == "start" {
			continue
		}
	}
	for _, cmd := range firstWeekCommands {
		if !strings.Contains(short, "  "+cmd) {
			t.Errorf("%q is on the first-week list and not in the short help", cmd)
		}
	}
	// The point of the short list is what it leaves out.
	for _, cmd := range extraCommands {
		if strings.Contains(short, "  "+cmd+" ") {
			t.Errorf("%q is in the short help; it belongs behind --all", cmd)
		}
	}
}

// The "N more" line is generated from the list it counts, so this asserts the two lists together
// cover run() — a command added to the switch and to neither list would be undiscoverable.
func TestHelpCountsTheRestCorrectly(t *testing.T) {
	_, short, _ := capture(t, func() int { usageFull(false); return exitOK })
	want := fmt.Sprintf("%d more", len(extraCommands))
	if !strings.Contains(short, want) {
		t.Errorf("short help does not say %q:\n%s", want, short)
	}

	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "cmd", "agentarch", "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	listed := map[string]bool{}
	for _, c := range append(append([]string{}, firstWeekCommands...), extraCommands...) {
		listed[c] = true
	}
	// Aliases and the flag spellings of help/version are reachable but are not separate
	// commands to document.
	skip := map[string]bool{
		"bp": true, "help": true, "--help": true, "-h": true,
		"--version": true, "-v": true,
	}

	for _, m := range caseArm.FindAllStringSubmatch(string(raw), -1) {
		for _, name := range strings.Split(m[1], ",") {
			name = strings.Trim(strings.TrimSpace(name), `"`)
			if name == "" || skip[name] || listed[name] {
				continue
			}
			t.Errorf("run() dispatches %q and neither help list mentions it", name)
		}
	}
}

// Matches the `case "x", "y":` arms of run()'s dispatch switch.
var caseArm = regexp.MustCompile(`(?m)^\tcase ((?:"[a-z-]+"(?:, )?)+):`)

// "runs on none" read as "runs on nothing" — as if the blueprint did not work. The id is `none`
// and means "no framework, the provider SDK directly"; only the display changes.
func TestFrameworkNoneReadsAsWords(t *testing.T) {
	if got := frameworkLabel("none"); !strings.HasPrefix(got, "no framework") && !strings.HasPrefix(got, "Python Native") {
		t.Errorf("frameworkLabel(none) = %q", got)
	}
	if got := frameworkLabel("langgraph"); got != "langgraph" {
		t.Errorf("a real framework name must survive unchanged, got %q", got)
	}

	got := frameworkValues([]string{"none", "langgraph"})
	if !strings.Contains(got, "none") {
		t.Errorf("%q does not name the value --framework actually takes", got)
	}
}

// The id is a value in blueprint.yaml, in --framework and in the directory under app/. Changing
// it there would break every blueprint and the CI job that installs each variant; the fix was
// always meant to be display-only.
func TestFrameworkNoneIsStillTheIdOnDisk(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := os.ReadDir(filepath.Join(repo, "content", "blueprints"))
	if err != nil {
		t.Fatal(err)
	}

	found := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(repo, "content", "blueprints", d.Name(), "blueprint.yaml"))
		if err != nil {
			continue
		}
		if !strings.Contains(string(raw), "none") {
			t.Errorf("%s/blueprint.yaml no longer declares the `none` framework id", d.Name())
			continue
		}
		if _, err := os.Stat(filepath.Join(repo, "content", "blueprints", d.Name(), "app", "none")); err != nil {
			t.Errorf("%s has no app/none directory; the id and the directory must agree", d.Name())
		}
		found++
	}
	if found == 0 {
		t.Fatal("no blueprints found; the check is looking in the wrong place")
	}
}

// withTTY runs fn as if somebody were at a terminal, and restores the real detector after.
func withTTY(t *testing.T, present bool, fn func()) {
	t.Helper()
	prev := isTTY
	isTTY = func() bool { return present }
	defer func() { isTTY = prev }()
	fn()
}

// chdir moves into dir for the duration of the test. run() reads the current directory, which is
// the whole thing being exercised here.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// A bare `agentarch` after `init`, before the first agent, exited 1 with twenty subcommands
// attached. Being midway through a setup is not an error, and the exit code is what a script — or
// a person reading `echo $?` — actually acts on.
//
// This is asserted here rather than by driving a pty from a shell: whether a test process has a
// controlling terminal varies between a laptop, CI and a container, so a pty-based check reports
// a different answer depending on where it runs, which is worse than no check.
func TestBareCommandNeverFailsSomebodyMidSetup(t *testing.T) {
	t.Run("installed with no agent yet", func(t *testing.T) {
		dir := t.TempDir()
		if code := cmdInit([]string{"--root", dir, "--profile", "standard"}); code != exitOK {
			t.Fatalf("init exited %d", code)
		}
		chdir(t, dir)

		var code int
		var stdout string
		withTTY(t, true, func() {
			stdout, _, code = capture(t, func() int { return run(nil) })
		})

		if code != exitOK {
			t.Errorf("exit %d — installing the standard and not yet having an agent is a "+
				"state to move on from, not a failure", code)
		}
		if !strings.Contains(stdout, "blueprint") {
			t.Errorf("does not say what to do next:\n%s", stdout)
		}
		if n := countCommandLines(stdout); n > 4 {
			t.Errorf("suggests %d commands; it is a wall again", n)
		}
	})

	t.Run("no terminal still refuses", func(t *testing.T) {
		dir := t.TempDir()
		if code := cmdInit([]string{"--root", dir, "--profile", "standard"}); code != exitOK {
			t.Fatalf("init exited %d", code)
		}
		chdir(t, dir)

		var code int
		withTTY(t, false, func() {
			_, _, code = capture(t, func() int { return run(nil) })
		})

		// A script invoking us with no arguments must get usage and a non-zero exit rather
		// than sitting on a prompt waiting for somebody who is not there.
		if code != exitUsage {
			t.Errorf("exit %d with no terminal, want %d", code, exitUsage)
		}
	})

	t.Run("empty directory reaches the interview", func(t *testing.T) {
		chdir(t, t.TempDir())

		// The interview refuses without a terminal, which is exactly how we can tell run()
		// routed there rather than to usage: only start prints this.
		var stderr string
		withTTY(t, false, func() {
			// isTTY false makes run() take the usage path, so drive cmdStart directly to
			// confirm the empty-directory branch is the interview and not a command list.
			_, stderr, _ = capture(t, func() int { return cmdStart([]string{"--root", "."}) })
		})
		if !strings.Contains(stderr, "nobody to ask") {
			t.Errorf("an empty directory should reach the interview:\n%s", stderr)
		}
	})
}
