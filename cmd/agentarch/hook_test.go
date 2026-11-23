package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hookProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agentarch", "project", "evidence"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func toolCall(t *testing.T, tool string, input map[string]any) hookInput {
	t.Helper()
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return hookInput{ToolName: tool, ToolInput: b}
}

func TestPreToolDeniesGeneratedFilesAndEvidence(t *testing.T) {
	root := hookProject(t)
	generated := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(generated, []byte("<!-- agentarch:generated from agentarch/std/core -->\n# Rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(root, "app.js")
	if err := os.WriteFile(ordinary, []byte("console.log(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   hookInput
		want string
	}{
		{"generated edit", toolCall(t, "Edit", map[string]any{"file_path": generated, "new_string": "x"}), "deny"},
		{"generated relative write", hookInput{CWD: root, ToolName: "Write", ToolInput: json.RawMessage(`{"file_path":"CLAUDE.md","content":"x"}`)}, "deny"},
		{"evidence export", toolCall(t, "Write", map[string]any{"file_path": filepath.Join(root, "agentarch/project/evidence/latest.json"), "content": "{}"}), "deny"},
		{"workflow state", toolCall(t, "Write", map[string]any{"file_path": filepath.Join(root, ".git/agentarch-workflow/state.json"), "content": "{}"}), "deny"},
		{"ordinary edit", toolCall(t, "Edit", map[string]any{"file_path": ordinary, "new_string": "console.log(2)"}), ""},
		{"new file", toolCall(t, "Write", map[string]any{"file_path": filepath.Join(root, "new.md"), "content": "# New"}), ""},
		{"other tool", toolCall(t, "Read", map[string]any{"file_path": generated}), ""},
	}
	for _, c := range cases {
		if got := preToolDecision(root, c.in).Verdict; got != c.want {
			t.Errorf("%s: verdict %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPreToolAsksBeforeWritingSecretShapedContent(t *testing.T) {
	root := hookProject(t)
	path := filepath.Join(root, "config.js")
	secrets := map[string]string{
		"aws":     "const k = 'AKIA" + strings.Repeat("A", 16) + "'",
		"github":  "token: ghp_" + strings.Repeat("a", 36),
		"pem":     "-----BEGIN RSA PRIVATE KEY-----\nabc",
		"stripe":  "sk_live_" + strings.Repeat("x", 24),
		"service": "eyJhbGciOiJIUzI1NiJ9.eyJyb2xlIjoic2VydmljZV9yb2xlIn0.c2ln",
	}
	for name, content := range secrets {
		d := preToolDecision(root, toolCall(t, "Write", map[string]any{"file_path": path, "content": content}))
		if d.Verdict != "ask" {
			t.Errorf("%s: verdict %q, want ask", name, d.Verdict)
		}
	}
	multi := toolCall(t, "MultiEdit", map[string]any{"file_path": path, "edits": []map[string]string{{"new_string": "ok"}, {"new_string": "AKIA" + strings.Repeat("B", 16)}}})
	if preToolDecision(root, multi).Verdict != "ask" {
		t.Error("a secret in the second of several edits was not caught")
	}
	for _, safe := range []string{"process.env.API_KEY", "const key = getenv('OPENAI_API_KEY')", "sk-short"} {
		if d := preToolDecision(root, toolCall(t, "Write", map[string]any{"file_path": path, "content": safe})); d.Verdict != "" {
			t.Errorf("%q: verdict %q, want allow", safe, d.Verdict)
		}
	}
}

func TestPreToolAsksTheOwnerBeforePublishingOrDeploying(t *testing.T) {
	ask := []string{
		"git push origin main",
		"git -C app push",
		"/usr/bin/git push --force",
		"npm test && git push",
		"gh pr merge 12 --squash",
		"vercel deploy --prod",
		"FOO=1 npx wrangler deploy",
		"sudo docker push ghcr.io/x/y:1",
		"terraform apply -auto-approve",
		"supabase db push",
		"git commit --no-verify -m x",
		"echo $(git push)",
	}
	for _, c := range ask {
		if d := commandDecision(c); d.Verdict != "ask" {
			t.Errorf("%q: verdict %q, want ask", c, d.Verdict)
		}
	}
	allow := []string{
		"git status", "git commit -m 'push the button'", "git log --oneline", "npm run build",
		"echo push", "agentarch workflow status", "cat .git/agentarch-workflow/state.json",
		"grep -r deploy docs", "terraform plan",
	}
	for _, c := range allow {
		if d := commandDecision(c); d.Verdict != "" {
			t.Errorf("%q: verdict %q, want allow", c, d.Verdict)
		}
	}
	for _, c := range []string{"echo {} > .git/agentarch-workflow/state.json", "rm agentarch/project/evidence/latest.json", "sed -i s/a/b/ agentarch/project/evidence/latest.json"} {
		if d := commandDecision(c); d.Verdict != "deny" {
			t.Errorf("%q: verdict %q, want deny", c, d.Verdict)
		}
	}
}

func TestPreToolFailsClosedOnUnreadableInput(t *testing.T) {
	root := hookProject(t)
	d := preToolDecision(root, hookInput{ToolName: "Write", ToolInput: json.RawMessage(`"not an object"`)})
	if d.Verdict != "deny" {
		t.Fatalf("unreadable tool input: verdict %q, want deny", d.Verdict)
	}
}

func TestClaudeProtocolOutput(t *testing.T) {
	var b bytes.Buffer
	emitClaude(&b, "pre-tool", decision{}, "")
	if b.Len() != 0 {
		t.Fatalf("an allow must print nothing, got %q", b.String())
	}
	emitClaude(&b, "pre-tool", decision{Verdict: "deny", Reason: "no"}, "")
	var out struct {
		H struct {
			Event, Decision, Reason string
		}
	}
	var raw map[string]map[string]string
	if err := json.Unmarshal(b.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	out.H.Event, out.H.Decision, out.H.Reason = raw["hookSpecificOutput"]["hookEventName"], raw["hookSpecificOutput"]["permissionDecision"], raw["hookSpecificOutput"]["permissionDecisionReason"]
	if out.H.Event != "PreToolUse" || out.H.Decision != "deny" || out.H.Reason != "no" {
		t.Fatalf("unexpected PreToolUse output: %s", b.String())
	}
	b.Reset()
	emitClaude(&b, "stop", decision{Verdict: "block", Reason: "fix"}, "")
	if !strings.Contains(b.String(), `"decision":"block"`) {
		t.Fatalf("stop block not in Claude's shape: %s", b.String())
	}
	b.Reset()
	emitClaude(&b, "session-start", decision{}, "ctx")
	if !strings.Contains(b.String(), `"additionalContext":"ctx"`) || !strings.Contains(b.String(), "SessionStart") {
		t.Fatalf("session start not in Claude's shape: %s", b.String())
	}
}

func TestSessionContextIsShortAndNamesTheNextStep(t *testing.T) {
	root := hookProject(t)
	got := sessionContext(root)
	if !strings.Contains(got, "agentarch start") {
		t.Fatalf("uninitialized project context does not say how to begin: %q", got)
	}
	if len(got) > 2000 {
		t.Fatalf("session context is %d bytes; it must stay compact", len(got))
	}
}

func TestEveryHookMessageExistsInEveryLanguage(t *testing.T) {
	for key := range hookMessages["en"] {
		for _, l := range languages {
			if _, ok := hookMessages[l.Code][key]; !ok {
				t.Errorf("hook message %q missing in %s", key, l.Code)
			}
		}
	}
	for key := range doctorMessages["en"] {
		for _, l := range languages {
			if _, ok := doctorMessages[l.Code][key]; !ok {
				t.Errorf("doctor message %q missing in %s", key, l.Code)
			}
		}
	}
}

func TestDoctorNamesEveryGapWithAFix(t *testing.T) {
	root := t.TempDir()
	none := func(string) (string, error) { return "", errors.New("not found") }
	fail := func(string, string, ...string) error { return errors.New("failed") }
	r := runDoctor(root, "linux", none, fail)
	if r.Ready {
		t.Fatal("an empty machine reported ready")
	}
	ids := map[string]bool{}
	for _, c := range r.Checks {
		ids[c.ID] = true
		if !c.OK && (c.Why == "" || c.Fix == "") {
			t.Errorf("%s fails without saying why and how to fix it", c.ID)
		}
	}
	for _, want := range []string{"git", "container-runtime", "workflow", "agentarch-on-path", "assistant"} {
		if !ids[want] {
			t.Errorf("doctor did not check %s", want)
		}
	}

	all := func(name string) (string, error) {
		if name == "container" || name == "docker" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}
	ok := func(string, string, ...string) error { return nil }
	r = runDoctor(root, "darwin", all, ok)
	for _, c := range r.Checks {
		if c.ID == "container-runtime" && (!c.OK || !strings.Contains(c.Detail, "podman")) {
			t.Errorf("podman on PATH not reported: %+v", c)
		}
		if c.ID == "git" && !c.OK {
			t.Errorf("git repository not recognised: %+v", c)
		}
	}
}
