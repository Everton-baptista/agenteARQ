package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Everton-baptista/agenteARQ/internal/plugin"
)

func fixturePlugin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	_, e := plugin.Install(root, "codex", plugin.Options{Owner: "Example Owner", Contact: "owner@example.invalid", ModelProvider: "test-provider", ModelID: "test-model-2026-11-01"})
	if e != nil {
		t.Fatal(e)
	}
	return root
}
func writeArtifact(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(body), 0644); e != nil {
		t.Fatal(e)
	}
}
func TestCheckAgentsUsesRegulatedPolicy(t *testing.T) {
	root := fixturePlugin(t)
	writeArtifact(t, root, "agentarch/agentarch.yaml", "schema_version: '1.0'\nproject:\n  default_profile: regulated\n")
	checks, e := CheckAgents(context.Background(), root, []string{"agentarch-codex"})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range checks {
		if c.ID == "control.ai.obs.otel_enabled" {
			found = true
			if c.Status == "passed" {
				t.Fatal("unconfigured telemetry passed")
			}
		}
	}
	if !found {
		t.Fatalf("regulated controls silently skipped: %+v", checks)
	}
}
func TestCheckAgentsRejectsUnknownProfile(t *testing.T) {
	root := fixturePlugin(t)
	writeArtifact(t, root, "agentarch/agentarch.yaml", "project:\n  default_profile: invented\n")
	if _, e := CheckAgents(context.Background(), root, []string{"agentarch-codex"}); e == nil {
		t.Fatal("unknown profile accepted")
	}
}
func TestCheckAgentsCancellationBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := CheckAgents(ctx, "does-not-exist", []string{"builder"}); e != context.Canceled {
		t.Fatalf("expected cancellation: %v", e)
	}
}
func TestCheckAgentsReportsMalformedArtifact(t *testing.T) {
	root := fixturePlugin(t)
	writeArtifact(t, root, "agentarch/project/agents/agentarch-codex/agent.yaml", "schema_version: '1.0'\nagent:\n  id: agentarch-codex\n")
	checks, e := CheckAgents(context.Background(), root, []string{"agentarch-codex"})
	if e != nil {
		t.Fatal(e)
	}
	if len(checks) == 0 || checks[0].Status != "failed" {
		t.Fatal("malformed agent did not fail")
	}
}
func TestArtifactReferencesStayInsideProject(t *testing.T) {
	root := t.TempDir()
	writeArtifact(t, root, "agentarch/project/agents/builder/agent.yaml", "agent:\n  prompts:\n    system:\n      path: ../../../../../outside.txt\n")
	if e := confineArtifacts(root); e == nil || !strings.Contains(e.Error(), "escapes") {
		t.Fatalf("external reference accepted: %v", e)
	}
}
func TestPolicyArtifactsRejectSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	if e := os.WriteFile(outside, []byte("secret: fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "agentarch", "project")
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(dir, "agent.yaml")); e != nil {
		t.Skip(e)
	}
	if e := confineArtifacts(root); e == nil {
		t.Fatal("symlink artifact accepted")
	}
}
