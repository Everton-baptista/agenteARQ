package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A project that uses no AI library at all must come back empty. Each file below once produced a
// false finding: "diagnostic" read as agno, any "agents" as the OpenAI Agents SDK, and a
// virtualenv under an unusual name contributed a library's model table.
func TestScanReportsNothingForAProjectWithoutAgents(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"app/diagnostics.py":   "def run_diagnostic():\n    \"\"\"Diagnose the agents in the fleet.\"\"\"\n",
		"config/settings.yaml": "user_agents:\n  - curl\nnotes: we may evaluate openai later\n",
		"requirements.txt":     "fastapi==0.115.0\nagentsmith-utils==1.0\nopenai-compat-shim==0.1\n",
		"env/pyvenv.cfg":       "home = /usr/bin\n",
		"env/lib/python3.12/site-packages/litellm/models.py": "DEFAULT = \"gpt-4.1\"\nfrom openai import OpenAI\n",
		"package.json":         `{"dependencies": {"react": "18.3.1"}, "description": "agents dashboard"}`,
		"src/agents/router.ts": "import { route } from \"./agents/route\";\n",
	})
	s := scanForAgents(root)
	if len(s.Providers)+len(s.Models)+len(s.Frameworks) != 0 {
		t.Fatalf("found evidence in a project without any: providers=%v models=%v frameworks=%v where=%v",
			s.Providers, s.Models, s.Frameworks, s.Where)
	}
}

func TestScanFindsDeclaredAndImportedLibrariesWithTheirFile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"requirements.txt": "anthropic==0.40.0\nagno[all]>=1.2\n",
		"app/main.py":      "from anthropic import Anthropic\nMODEL = \"claude-sonnet-4-5-20250929\"\n",
		"web/package.json": `{"dependencies": {"@openai/agents": "0.1.0", "ai": "4.0.0"}}`,
		"web/src/chat.ts":  "import { Agent } from \"@openai/agents\";\n",
	})
	s := scanForAgents(root)
	if want := []string{"anthropic"}; !reflect.DeepEqual(s.Providers, want) {
		t.Errorf("providers %v, want %v", s.Providers, want)
	}
	if want := []string{"agno", "openai-agents-sdk", "vercel-ai-sdk"}; !reflect.DeepEqual(s.Frameworks, want) {
		t.Errorf("frameworks %v, want %v", s.Frameworks, want)
	}
	if want := []string{"claude-sonnet-4-5-20250929"}; !reflect.DeepEqual(s.Models, want) {
		t.Errorf("models %v, want %v", s.Models, want)
	}
	if got := s.withWhere("framework", s.Frameworks, 5); !strings.Contains(got, "agno (requirements.txt)") {
		t.Errorf("finding without its file: %s", got)
	}
}

// A model string says what someone typed, not what runs in production, so the adopted manifest
// lists it as a candidate and leaves model.id unknown.
func TestAdoptedManifestNeverFillsTheModelFromAScan(t *testing.T) {
	root := t.TempDir()
	s := adoptScan{Providers: []string{"openai"}, Models: []string{"gpt-4.1"},
		Where: map[string]string{"model:gpt-4.1": "tests/fixtures/chat.py"}}
	if err := writeAdoptedManifest(root, "legacy", s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, "agentarch", "project", "agents", "legacy", "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "gpt-4.1 (tests/fixtures/chat.py)") {
		t.Error("the candidate model and its file are not shown")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "id:") && strings.Contains(line, "gpt-4.1") {
			t.Fatalf("model.id was filled from a scan: %q", line)
		}
	}
}
