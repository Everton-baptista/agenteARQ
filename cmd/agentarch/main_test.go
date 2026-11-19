package main

// Smoke tests for the command layer. Everything runs against t.TempDir() roots and the
// embedded content payload — no network, no git, no terminal. The commands write directly
// to os.Stdout/os.Stderr, so output assertions go through capture, which swaps both for
// pipes around the call.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture runs fn with os.Stdout and os.Stderr redirected to pipes and returns what was
// written to each, plus fn's exit code. The reads run concurrently so a chatty command
// cannot fill the pipe buffer and block on a write nobody is draining.
func capture(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()

	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	outCh := make(chan []byte, 1)
	errCh := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(rOut); outCh <- b }()
	go func() { b, _ := io.ReadAll(rErr); errCh <- b }()

	os.Stdout, os.Stderr = wOut, wErr
	code = fn()
	os.Stdout, os.Stderr = oldOut, oldErr

	if err := wOut.Close(); err != nil {
		t.Fatalf("closing stdout pipe: %v", err)
	}
	if err := wErr.Close(); err != nil {
		t.Fatalf("closing stderr pipe: %v", err)
	}
	stdout, stderr = string(<-outCh), string(<-errCh)
	return stdout, stderr, code
}

// startNewProject runs the full non-interactive start flow in a fresh temp dir and returns
// the project root and its combined output. Every answer is passed as a flag, which is the
// only form that works without a terminal.
func startNewProject(t *testing.T, owner string) (root, output string) {
	t.Helper()
	root = t.TempDir()

	args := []string{
		"--root", root,
		"--new",
		"--blueprint", "rag-support",
		"--framework", "none",
		"--jurisdictions", "BR",
		"--yes",
	}
	if owner != "" {
		args = append(args, "--owner", owner)
	}

	stdout, stderr, code := capture(t, func() int { return cmdStart(args) })
	if code != exitOK {
		t.Fatalf("start --new exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	return root, stdout + stderr
}

func TestStartNewEndToEnd(t *testing.T) {
	root, out := startNewProject(t, "Test Person")

	for _, p := range []string{
		"agentarch/std",
		"agentarch/project",
		"app",
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("expected %s in the generated project: %v", p, err)
		}
	}

	// The closing instructions must name the entry point that actually exists.
	if !strings.Contains(out, "python -m app.cli") {
		t.Errorf("closing output should tell the user to run `python -m app.cli`, got:\n%s", out)
	}
	if strings.Contains(out, "app/agent.py") {
		t.Errorf("closing output must not reference the nonexistent app/agent.py, got:\n%s", out)
	}

	// --owner was passed, so the closing text must not claim owner.accountable still names
	// the blueprint's example person.
	if strings.Contains(out, "example person") {
		t.Errorf("output claims the owner is still the blueprint's example person despite --owner, got:\n%s", out)
	}

	// The project just produced must validate clean.
	stdout, stderr, code := capture(t, func() int { return cmdValidate([]string{"--root", root}) })
	if code != exitOK {
		t.Errorf("validate on the started project exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}
}

func TestValidateOnStartedProject(t *testing.T) {
	root, _ := startNewProject(t, "")

	stdout, stderr, code := capture(t, func() int { return cmdValidate([]string{"--root", root}) })
	if code != exitOK {
		t.Fatalf("validate exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}

func TestStartWithoutFlagsRefusesWhenNotATerminal(t *testing.T) {
	// Bare `start` with no flags and no TTY must refuse with a usage error rather than
	// hanging on a question nobody can answer. Under `go test`, stdin is /dev/null, so
	// isTTY() is false.
	_, stderr, code := capture(t, func() int { return cmdStart([]string{"--root", t.TempDir()}) })
	if code != exitUsage {
		t.Fatalf("bare start without a terminal exited %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "not a terminal") {
		t.Errorf("stderr should explain there is nobody to ask, got:\n%s", stderr)
	}
}
