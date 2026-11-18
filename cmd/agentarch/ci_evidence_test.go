package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGateEvidenceRejectsCommentsAndIgnoredFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       bool
	}{
		{"real", "run: /tmp/agentarch check --profile standard", true},
		{"comment", "run: |\n          # agentarch check\n          echo ok", false},
		{"echo", "run: echo agentarch check", false},
		{"ignored", "continue-on-error: true\n        run: agentarch check", false},
		{"or-true", "run: agentarch check || true", false},
		{"conditional", "if: 'false'\n        run: agentarch check", false},
		{"dead-branch", "run: |\n          if false; then\n            agentarch check\n          fi", false},
		{"function-only", "run: |\n          check() {\n            agentarch check\n          }", false},
		{"heredoc", "run: |\n          cat <<EOF\n          agentarch check\n          EOF", false},
		{"custom-shell", "shell: bash {0}\n        run: |\n          agentarch check\n          true", false},
		{"errexit-disabled", "run: |\n          set +o errexit\n          agentarch check\n          true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, ".github", "workflows")
			_ = os.MkdirAll(dir, 0755)
			_ = os.WriteFile(filepath.Join(dir, "check.yml"), []byte("jobs:\n  check:\n    steps:\n      - "+tc.body+"\n"), 0644)
			if got := gateRunsInCI(root); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
