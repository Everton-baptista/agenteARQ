package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeInstallWritesOwnedHooksAndKeepsUserSettings(t *testing.T) {
	root := t.TempDir()
	user := `{
  "permissions": {"allow": ["Bash(npm test)"]},
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./my-lint.sh"}]}]
  }
}
`
	writeTest(t, root, ".claude/settings.json", user)
	if _, err := Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	b := readTest(t, root, ".claude/settings.json")
	var settings struct {
		Permissions map[string][]string `json:"permissions"`
		Hooks       map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Permissions["allow"]) != 1 {
		t.Fatal("user permissions lost")
	}
	pre := settings.Hooks["PreToolUse"]
	if len(pre) != 2 || pre[0].Hooks[0].Command != "./my-lint.sh" || !strings.Contains(pre[1].Hooks[0].Command, "hook --event pre-tool --host claude") {
		t.Fatalf("PreToolUse groups wrong: %s", b)
	}
	for _, event := range []string{"SessionStart", "Stop"} {
		if len(settings.Hooks[event]) != 1 {
			t.Fatalf("%s hook not installed: %s", event, b)
		}
	}
	if report, err := Check(root, "claude"); err != nil || len(report.Issues) > 0 {
		t.Fatalf("fresh install reports drift: %v %v", err, report.Issues)
	}
	// Reinstalling is idempotent.
	if _, err := Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	if string(readTest(t, root, ".claude/settings.json")) != string(b) {
		t.Fatal("reinstall changed settings")
	}
	// Removal leaves exactly the user's settings.
	if _, err := Remove(root, "claude", Options{}); err != nil {
		t.Fatal(err)
	}
	after := readTest(t, root, ".claude/settings.json")
	if strings.Contains(string(after), "hook --event") || !strings.Contains(string(after), "./my-lint.sh") || !strings.Contains(string(after), "npm test") {
		t.Fatalf("remove did not restore the user's settings: %s", after)
	}
}

func TestClaudeHooksFileCreatedByInstallIsDeletedByRemove(t *testing.T) {
	root := t.TempDir()
	if _, err := Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(root, "claude", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("settings.json created by install survived remove")
	}
}

func TestChangedManagedHooksAreAConflict(t *testing.T) {
	root := t.TempDir()
	if _, err := Install(root, "claude", testOptions()); err != nil {
		t.Fatal(err)
	}
	b := readTest(t, root, ".claude/settings.json")
	tampered := strings.Replace(string(b), "--event stop", "--event stop --quiet", 1)
	writeTest(t, root, ".claude/settings.json", tampered)
	report, err := Check(root, "claude")
	if err != nil || len(report.Issues) == 0 {
		t.Fatalf("tampered hooks not reported: %v %v", err, report.Issues)
	}
	if _, err := Install(root, "claude", testOptions()); err == nil {
		t.Fatal("install overwrote changed managed hooks")
	}
}

func TestShellQuoteOnlyWhenNeeded(t *testing.T) {
	for in, want := range map[string]string{
		"agentarch":                "agentarch",
		"/usr/local/bin/agentarch": "/usr/local/bin/agentarch",
		"/Users/a b/agentarch":     `'/Users/a b/agentarch'`,
		"it's":                     `'it'\''s'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
