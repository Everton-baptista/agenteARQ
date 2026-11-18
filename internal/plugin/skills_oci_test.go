package plugin

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

func TestInstalledSkillPythonLaunchContract(t *testing.T) {
	for _, host := range Hosts() {
		t.Run(host, func(t *testing.T) {
			root := t.TempDir()
			if _, err := Install(root, host, testOptions()); err != nil {
				t.Fatal(err)
			}
			descriptor, err := Describe(host)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			err = filepath.WalkDir(filepath.Join(root, descriptor.SkillsPath), func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() || filepath.Ext(path) != ".py" {
					return err
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if bytes.HasPrefix(body, []byte("#!")) != (info.Mode().Perm()&0111 != 0) {
					t.Errorf("installed Python launch contract disagrees with file permissions: %s (%v)", path, info.Mode().Perm())
				}
				count++
				return nil
			})
			if err != nil || count == 0 {
				t.Fatalf("installed Python helpers: count=%d error=%v", count, err)
			}
		})
	}
}

// Install the embedded payload, then lint the actual staged file modes. Checking
// only content/skills would miss permissions lost by packaging or installation.
func TestInstalledSkillExecutabilityOCI(t *testing.T) {
	image := os.Getenv("AGENTARCH_TEST_PYTHON_VERIFIER_IMAGE")
	if image == "" {
		t.Skip("set AGENTARCH_TEST_PYTHON_VERIFIER_IMAGE to an installed digest-pinned Python verifier")
	}
	for _, host := range Hosts() {
		t.Run(host, func(t *testing.T) {
			root := t.TempDir()
			if output, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
				t.Fatalf("git init: %v: %s", err, output)
			}
			if _, err := Install(root, host, testOptions()); err != nil {
				t.Fatal(err)
			}
			descriptor, err := Describe(host)
			if err != nil {
				t.Fatal(err)
			}
			// Explicit target and rule selection make this regression independent of
			// Ruff's default rules and hidden-directory discovery. Production keeps
			// its full lint command and does not gain any skips or exclusions.
			command := workflow.Command{ID: "skill-executability", Program: "python", Args: []string{
				"-m", "ruff", "check", "--no-cache", "--select", "EXE001,EXE002", descriptor.SkillsPath,
			}, Timeout: 30 * time.Second}
			runner := &workflow.ContainerRunner{Image: image, DiagnosticsReason: "Synthetic installed skills and executable-mode fixtures."}
			result := runner.Run(context.Background(), root, command)
			if result.Status != "passed" || result.ExitCode != 0 {
				t.Fatalf("installed skill modes violate executable rules: %+v", result)
			}
			commands, err := workflow.ProfileCommands("python-api")
			if err != nil {
				t.Fatal(err)
			}
			for _, lint := range commands {
				if lint.ID == "lint" {
					result = runner.Run(context.Background(), root, lint)
					if result.Status != "passed" || result.ExitCode != 0 {
						t.Fatalf("full profile lint failed for installed skills: %+v", result)
					}
				}
			}
			// Both negative controls must survive installation's staging/tar path.
			// The valid executable catches an accidental chmod-to-0644 in staging.
			if host == "codex" {
				for _, fixture := range []struct {
					name string
					body string
					mode os.FileMode
				}{
					{"broken_nonexec.py", "#!/usr/bin/env python3\nprint('fixture')\n", 0644},
					{"broken_executable.py", "print('fixture')\n", 0755},
					{"correct_executable.py", "#!/usr/bin/env python3\nprint('fixture')\n", 0755},
				} {
					path := filepath.Join(root, descriptor.SkillsPath, fixture.name)
					if err := os.WriteFile(path, []byte(fixture.body), fixture.mode); err != nil {
						t.Fatal(err)
					}
				}
				result = runner.Run(context.Background(), root, command)
				if result.Status != "failed" || result.ExitCode != 1 || result.OutputTruncated {
					t.Fatalf("inconsistent executable modes passed: %+v", result)
				}
				for _, want := range []string{"EXE001", "--> " + descriptor.SkillsPath + "/broken_nonexec.py:", "EXE002", "--> " + descriptor.SkillsPath + "/broken_executable.py:"} {
					if !strings.Contains(result.OutputExcerpt, want) {
						t.Errorf("missing %s in mode findings: %s", want, result.OutputExcerpt)
					}
				}
				if strings.Contains(result.OutputExcerpt, "--> "+descriptor.SkillsPath+"/correct_executable.py:") {
					t.Errorf("staging lost executable permission: %s", result.OutputExcerpt)
				}
			}
		})
	}
}
