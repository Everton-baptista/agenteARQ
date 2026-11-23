package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIWorkflowRunsTheIsolatedChecksAndNeverAdvances(t *testing.T) {
	body, err := CIWorkflow("static-site", "v0.6.0", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"agenteARQ/v0.6.0/scripts/install.sh", "prepare-verifier --engine docker", "agentarch workflow ci-verify",
		"permissions:\n  contents: read", "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("template lacks %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"checkpoint", "review", "record-", "secrets."} {
		if strings.Contains(strings.ToLower(strings.SplitN(body, "name: agentarch verify", 2)[1]), forbidden) {
			t.Errorf("CI template must not %s", forbidden)
		}
	}
	if _, err := CIWorkflow("universal", "main", ""); err == nil {
		t.Fatal("universal without a base was accepted; its toolchain must be chosen")
	}
	if _, err := CIWorkflow("universal", "main", "python:3.12"); err == nil {
		t.Fatal("an unpinned base was accepted")
	}
	if body, err := CIWorkflow("universal", "main", "docker.io/library/python:3.12-slim@sha256:"+strings.Repeat("a", 64)); err != nil || !strings.Contains(body, "--base docker.io/library/python") {
		t.Fatalf("pinned universal base not used: %v", err)
	}
}

func TestCISummaryNamesEveryCheck(t *testing.T) {
	got := CISummary(Verification{Passed: false, Checks: []CheckResult{{ID: "html", Status: "passed"}, {ID: "secrets", Status: "failed"}}})
	if !strings.Contains(got, "did not pass") || !strings.Contains(got, "| secrets | failed |") {
		t.Fatalf("summary: %s", got)
	}
}

func TestCITemplateNeverOverwrites(t *testing.T) {
	s := riskTree(t, map[string]string{CIWorkflowPath: "name: mine\n"})
	if err := os.MkdirAll(filepath.Join(s.Root, "agentarch", "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteCIWorkflow("main", ""); err == nil {
		t.Fatal("an existing workflow was overwritten or no error was returned")
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, CIWorkflowPath))
	if string(b) != "name: mine\n" {
		t.Fatal("the existing workflow changed")
	}
}

func TestBrowserChecksAreOptInAndStaticSiteOnly(t *testing.T) {
	base, err := commandsFor(Product{Profile: "static-site"})
	if err != nil {
		t.Fatal(err)
	}
	with, err := commandsFor(Product{Profile: "static-site", BrowserChecks: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(with) != len(base)+1 || with[len(with)-1].ID != "browser" {
		t.Fatalf("browser check not added: %v", with)
	}
	for _, c := range base {
		if c.ID == "browser" {
			t.Fatal("browser check runs without opting in")
		}
	}
	if _, err := commandsFor(Product{Profile: "docs", BrowserChecks: true}); err == nil {
		t.Fatal("browser_checks accepted for a profile without a browser recipe")
	}
}
