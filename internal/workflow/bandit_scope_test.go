package workflow

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This uses the actual pinned verifier and the actual profile argv. It deliberately
// keeps assert fixtures in production-like paths so a global B101 skip cannot pass.
func TestPythonBanditScopeOCI(t *testing.T) {
	image := os.Getenv("AGENTARCH_TEST_PYTHON_VERIFIER_IMAGE")
	if image == "" {
		t.Skip("set AGENTARCH_TEST_PYTHON_VERIFIER_IMAGE to an installed digest-pinned Python verifier")
	}
	commands, err := ProfileCommands("python-api")
	if err != nil {
		t.Fatal(err)
	}
	var security Command
	for _, c := range commands {
		if c.ID == "security" {
			security = c
		}
	}
	if security.Program == "" {
		t.Fatal("Python profile has no security check")
	}
	security.Args = append(append([]string{}, security.Args...), "-f", "json")
	security.Timeout = 30 * time.Second
	runner := &ContainerRunner{Image: image, DiagnosticsReason: "Synthetic assert fixtures verify Bandit scan boundaries."}
	excluded := []string{"tests/test_example.py", "tests/nested/test_deep.py", ".venv/lib/fixture.py"}
	assertCode := "def check(value):\n    assert value\n"
	for _, tc := range []struct {
		name     string
		flagged  []string
		wantExit int
	}{
		{name: "root_test_and_virtualenv_assertions_are_excluded", wantExit: 0},
		{name: "application_nested_tests_and_managed_skills_remain_scanned", wantExit: 1, flagged: []string{
			"app.py", "src/tests/test_nested.py", "tests_support/helper.py",
			".venv-backup/helper.py", ".agents/skills/example/scripts/check.py",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := repo(t)
			write(t, root, "app.py", "def health():\n    return True\n")
			for _, path := range excluded {
				write(t, root, path, "def test_fixture():\n    assert 1 == 1\n")
			}
			for _, path := range tc.flagged {
				write(t, root, path, assertCode)
			}
			// A user Git ignore must not hide the intentional .venv/scan fixtures.
			if output, err := exec.Command("git", "-C", root, "add", "--force", "--all").CombinedOutput(); err != nil {
				t.Fatalf("track scan fixtures: %v: %s", err, output)
			}
			if tc.wantExit == 0 {
				pytest := runner.Run(context.Background(), root, Command{ID: "test", Program: "python", Args: []string{"-m", "pytest", "-q", "tests"}, Timeout: 30 * time.Second})
				if pytest.Status != "passed" || pytest.ExitCode != 0 {
					t.Fatalf("pytest assertion fixture failed: %+v", pytest)
				}
			}
			result := runner.Run(context.Background(), root, security)
			if result.ExitCode != tc.wantExit || result.Status == "inconclusive" || result.OutputTruncated {
				t.Fatalf("Bandit result status=%s exit=%d truncated=%t: %s %s", result.Status, result.ExitCode, result.OutputTruncated, result.Message, result.OutputExcerpt)
			}
			var report struct {
				Errors  []json.RawMessage `json:"errors"`
				Results []struct {
					Filename string `json:"filename"`
					TestID   string `json:"test_id"`
				} `json:"results"`
				Metrics map[string]json.RawMessage `json:"metrics"`
			}
			// Docker may run on a remote clock a fraction of a second behind the
			// client. Only tar's known timestamp warnings may precede Bandit JSON.
			jsonStart := strings.Index(result.OutputExcerpt, "{")
			if jsonStart < 0 {
				t.Fatalf("Bandit emitted no JSON: %s", result.OutputExcerpt)
			}
			for _, line := range strings.Split(strings.TrimSpace(result.OutputExcerpt[:jsonStart]), "\n") {
				if line != "" && !(strings.HasPrefix(line, "tar: ") && strings.Contains(line, "time stamp") && strings.HasSuffix(line, " s in the future")) {
					t.Fatalf("unexpected output before Bandit JSON: %s", line)
				}
			}
			if err := json.Unmarshal([]byte(result.OutputExcerpt[jsonStart:]), &report); err != nil {
				t.Fatalf("Bandit JSON: %v: %s", err, result.OutputExcerpt)
			}
			if len(report.Errors) != 0 || len(report.Results) != len(tc.flagged) {
				t.Fatalf("expected %d findings, got %+v; scan errors=%s", len(tc.flagged), report.Results, report.Errors)
			}
			seen := map[string]bool{}
			for _, finding := range report.Results {
				if finding.TestID != "B101" {
					t.Fatalf("unexpected finding: %+v", finding)
				}
				seen[finding.Filename] = true
			}
			for _, path := range tc.flagged {
				if !seen["./"+path] {
					t.Errorf("application assert was not reported: %s", path)
				}
			}
			for _, path := range excluded {
				if _, scanned := report.Metrics["./"+path]; scanned {
					t.Errorf("declared root test/dependency directory was scanned: %s", path)
				}
			}
			if _, scanned := report.Metrics["./app.py"]; !scanned {
				t.Error("application file was not scanned")
			}
		})
	}
}
