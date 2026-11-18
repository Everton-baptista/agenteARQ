package policy_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

func fileCheck(t *testing.T, ctx map[string]any, expression string) policy.Result {
	t.Helper()
	c := policy.Control{ID: "control.ai.api.file_test", Check: policy.Check{Kind: "file_exists", PathExpr: expression, Message: "Required artifact is absent."}}
	cat := &policy.Catalog{Controls: map[string]policy.Control{c.ID: c}}
	results := policy.Evaluate(cat, []policy.Resolution{{ControlID: c.ID, Severity: policy.SevBlocker}}, ctx, nil, time.Now())
	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	return results[0]
}

func fileProject(t *testing.T) (string, string, map[string]any) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "agentarch/project/agents/example")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte("agent:\n  id: example\n  file: artifact.json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentarch/agentarch.yaml"), []byte("layout:\n  paths:\n    contract: contracts/api.json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	agents, err := policy.LoadAgents(root, time.Now())
	if err != nil || len(agents) != 1 {
		t.Fatalf("load agents: %v (%d)", err, len(agents))
	}
	return root, dir, agents[0].Ctx
}

func TestFileExistsRequiresVerifiedFileContext(t *testing.T) {
	for _, ctx := range []map[string]any{
		{"agent": map[string]any{"id": "test", "file": "artifact.json"}},
		{"_agentarch_file_context": map[string]any{"root": "/", "agentDir": "/"}},
	} {
		r := fileCheck(t, ctx, `"etc/passwd"`)
		if r.Passed || r.EvidenceVerified || r.Error == "" {
			t.Fatalf("unresolved or manifest-forged context passed: %+v", r)
		}
	}
}

func TestFileExistsChecksActualRegularFileAtEvaluationTime(t *testing.T) {
	_, dir, ctx := fileProject(t)
	path := filepath.Join(dir, "artifact.json")
	r := fileCheck(t, ctx, "agent.file")
	if r.Passed || r.EvidenceVerified || r.Error != "" || r.Message == "" {
		t.Fatalf("missing file: %+v", r)
	}
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	r = fileCheck(t, ctx, "agent.file")
	if !r.Passed || !r.EvidenceVerified || r.Error != "" {
		t.Fatalf("actual file failed: %+v", r)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	r = fileCheck(t, ctx, "agent.file")
	if r.Passed || r.EvidenceVerified {
		t.Fatalf("directory or stale file existence passed: %+v", r)
	}
}

func TestFileExistsConfinesPathsAndRejectsSymlinks(t *testing.T) {
	root, dir, ctx := fileProject(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	for _, expression := range []string{
		fmt.Sprintf("%q", outside), `"../../../../../outside.json"`,
		`"link.json"`, `"../../../../linked-dir/outside.json"`, "", "true", "[agent.file]", "agent.file[",
	} {
		t.Run(expression, func(t *testing.T) {
			r := fileCheck(t, ctx, expression)
			if r.Passed || r.EvidenceVerified || r.Error == "" {
				t.Fatalf("unsafe or invalid path accepted: %+v", r)
			}
		})
	}
}

func TestFileExistsResolvesConfiguredContractFromProjectRoot(t *testing.T) {
	root, _, ctx := fileProject(t)
	if err := os.MkdirAll(filepath.Join(root, "contracts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "contracts/api.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	r := fileCheck(t, ctx, "repository.contract_path")
	if !r.Passed || !r.EvidenceVerified || r.Error != "" {
		t.Fatalf("configured contract did not resolve: %+v", r)
	}
	if err := os.Remove(filepath.Join(root, "contracts/api.json")); err != nil {
		t.Fatal(err)
	}
	r = fileCheck(t, ctx, "repository.contract_path")
	if r.Passed || r.EvidenceVerified {
		t.Fatalf("removed configured contract passed: %+v", r)
	}
}
