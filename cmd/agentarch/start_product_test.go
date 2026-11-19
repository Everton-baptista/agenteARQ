package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	return root
}

// Any product, not only agents: start opens the product workflow and writes no agent.
func TestStartProductOpensTheWorkflowWithoutAnAgent(t *testing.T) {
	root := gitRepo(t)
	out, stderr, code := capture(t, func() int {
		return cmdStart([]string{"--root", root, "--product", "--name", "loja", "--goal", "Visitors find products and the shop's contact",
			"--owner", "Ana Silva", "--engineering-profile", "universal", "--yes"})
	})
	if code != exitOK {
		t.Fatalf("start --product exited %d: %s", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(root, "agentarch", "project", "product.yaml"))
	if err != nil || !strings.Contains(string(b), "profile: universal") || !strings.Contains(string(b), "author: Ana Silva") {
		t.Fatalf("product.yaml not written as asked: %v\n%s", err, b)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "agentarch", "project", "agents")); len(entries) > 0 {
		t.Fatalf("a product start wrote an agent: %v", entries)
	}
	adr, _ := os.ReadFile(filepath.Join(root, "agentarch", "project", "adr", "0001-architecture.md"))
	if !strings.Contains(string(adr), "## Options considered") {
		t.Fatal("the ADR template does not ask for options to compare")
	}
	if !strings.Contains(out, "not chosen yet") {
		t.Errorf("the plan should say the technology is not chosen yet:\n%s", out)
	}
}

func TestStartProductChoosesNoProfileForAScript(t *testing.T) {
	root := gitRepo(t)
	_, stderr, code := capture(t, func() int {
		return cmdStart([]string{"--root", root, "--product", "--goal", "Visitors find products and the shop's contact",
			"--owner", "Ana Silva", "--yes"})
	})
	if code == exitOK {
		t.Fatal("start --product picked a profile on its own")
	}
	for _, want := range []string{"--engineering-profile", "universal", "checks:"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("the refusal should compare the profiles; missing %q:\n%s", want, stderr)
		}
	}
}

// The standard installed into a project carries no blueprints: they were verified as the
// product's own code. Installing one still works, from the binary.
func TestInitLeavesBlueprintsInTheBinary(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := capture(t, func() int { return cmdInit([]string{"--root", root}) }); code != exitOK {
		t.Fatalf("init: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "agentarch", "std", "blueprints")); err == nil {
		t.Fatal("init copied the blueprints into agentarch/std")
	}
	_, stderr, code := capture(t, func() int {
		return cmdBlueprintAdd([]string{"--root", root, "--id", "tool-approval", "--provider", "google", "--yes"})
	})
	if code != exitOK {
		t.Fatalf("blueprint add after init: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "requirements.txt")); err != nil {
		t.Fatal("blueprint add wrote no app")
	}
}
