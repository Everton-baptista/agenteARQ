package main

// Smoke tests for init, check and upgrade wiring: flag validation, what reaches the config
// file, and which version `upgrade` announces.

import (
	"io/fs"
	"os"
	"path/filepath"
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
