package validate_test

import (
	"os"
	"path/filepath"
	"testing"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/validate"
	"gopkg.in/yaml.v3"
)

// repoRoot walks up from the package directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		d = filepath.Dir(d)
	}
	t.Fatal("module root not found")
	return ""
}

func newValidator(t *testing.T) *validate.Validator {
	t.Helper()
	v, err := validate.New(agentarch.Spec)
	if err != nil {
		t.Fatalf("schemas do not compile: %v", err)
	}
	return v
}

// TestPassingExamples guards against the schemas becoming so strict that a reasonable, complete
// agent cannot satisfy them. A standard nobody can comply with is not a strict standard, it is
// an unused one.
func TestPassingExamples(t *testing.T) {
	root := repoRoot(t)
	v := newValidator(t)

	dirs, err := filepath.Glob(filepath.Join(root, "examples", "0*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no passing examples found: %v", err)
	}

	for _, d := range dirs {
		t.Run(filepath.Base(d), func(t *testing.T) {
			findings, err := v.Project(d)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range findings {
				t.Errorf("unexpected finding: %s", f)
			}
		})
	}
}

// expectation mirrors examples/99-failing/<case>/expected.yaml.
type expectation struct {
	Case   string `yaml:"case"`
	Phase  int    `yaml:"phase"`
	Expect struct {
		ExitCode *int `yaml:"exit_code"`
		Validate *struct {
			ExitCode int `yaml:"exit_code"`
		} `yaml:"validate"`
	} `yaml:"expect"`
}

// TestFailingExamples asserts the failure, not the success. Without these, a refactor that
// quietly stops enforcing a rule looks exactly like a passing build.
func TestFailingExamples(t *testing.T) {
	root := repoRoot(t)
	v := newValidator(t)

	files, err := filepath.Glob(filepath.Join(root, "examples", "99-failing", "*", "expected.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no failing examples found: %v", err)
	}

	for _, f := range files {
		dir := filepath.Dir(f)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var exp expectation
			if err := yaml.Unmarshal(raw, &exp); err != nil {
				t.Fatal(err)
			}

			// Determine the exit code validate is expected to produce. A case may
			// declare it directly, or nest it under `validate:` when the case is really
			// about the gate (phase 2) and passes validation on purpose.
			want := 2
			switch {
			case exp.Expect.Validate != nil:
				want = exp.Expect.Validate.ExitCode
			case exp.Expect.ExitCode != nil:
				want = *exp.Expect.ExitCode
			}

			findings, err := v.Project(dir)
			if err != nil {
				t.Fatal(err)
			}

			got := 0
			if len(findings) > 0 {
				got = 2
			}

			if got != want {
				if want == 2 {
					t.Fatalf("case %q must fail validation but passed — the rule it documents is not enforced", exp.Case)
				}
				t.Fatalf("case %q must pass validation (it is a phase-%d gate case) but produced %d finding(s): %v",
					exp.Case, exp.Phase, len(findings), findings)
			}
		})
	}
}
