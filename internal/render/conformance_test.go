package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Everton-baptista/agenteARQ/internal/render"
	"gopkg.in/yaml.v3"
)

func moduleRoot(t *testing.T) string {
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

type budgetFile struct {
	CoreBudget int `yaml:"core_budget"`
	Targets    map[string]struct {
		Path   string `yaml:"path"`
		Budget int    `yaml:"budget"`
	} `yaml:"targets"`
	Cases []struct {
		ID         string `yaml:"id"`
		About      string `yaml:"about"`
		Target     string `yaml:"target"`
		CoreBytes  any    `yaml:"core_bytes"`
		Expect     string `yaml:"expect"`
		WritesFile *bool  `yaml:"writes_file"`
	} `yaml:"cases"`
}

// renderOverhead measures what a target adds to the core: the generated header, any front
// matter, and the trailing newline. It is measured rather than hard-coded so the fixture pins
// the rule — the rendered file is what the budget bounds — without pinning this implementation's
// header wording, which is not part of the contract.
func renderOverhead(t *testing.T, tg render.Target) int {
	t.Helper()
	const probe = 16
	core := render.Core{
		Text:   strings.Repeat("x", probe),
		SHA256: strings.Repeat("a", 64),
		Lang:   "en",
	}
	out, err := render.Render(tg, core, "1.0.0", "")
	if err != nil {
		t.Fatalf("a %d byte core does not render for %s: %v", probe, tg.Name, err)
	}
	return len(out) - probe
}

// TestSpecConformanceBudgets runs spec/conformance/budgets against this implementation.
//
// The rule is one sentence — "an implementation MUST fail rather than truncate" — and it is the
// most tempting MUST in the specification to get wrong, because truncating is what every other
// tool does with an oversized buffer and it produces no visible error. An assistant handed half
// a rulebook follows half the rules and reports nothing.
func TestSpecConformanceBudgets(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "spec", "conformance", "budgets", "cases.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f budgetFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("budget conformance suite is empty")
	}

	// The fixture's numbers must be the implementation's. A suite that agreed only with itself
	// would pin nothing.
	if f.CoreBudget != render.BudgetCore {
		t.Errorf("fixture core budget is %d, the implementation uses %d",
			f.CoreBudget, render.BudgetCore)
	}
	for name, want := range f.Targets {
		tg, ok := render.TargetByName(name)
		if !ok {
			t.Errorf("the fixture names target %q, which the implementation does not have", name)
			continue
		}
		if tg.Budget != want.Budget {
			t.Errorf("%s budget is %d, the fixture says %d", name, tg.Budget, want.Budget)
		}
		if tg.Path != want.Path {
			t.Errorf("%s path is %s, the fixture says %s", name, tg.Path, want.Path)
		}
	}

	for _, c := range f.Cases {
		t.Run(c.ID, func(t *testing.T) {
			tg, ok := render.TargetByName(c.Target)
			if !ok {
				t.Fatalf("unknown target %q", c.Target)
			}

			var n int
			switch v := c.CoreBytes.(type) {
			case int:
				n = v
			case string:
				switch v {
				case "renders_at_budget":
					n = tg.Budget - renderOverhead(t, tg)
				case "renders_one_over":
					n = tg.Budget - renderOverhead(t, tg) + 1
				case "core_equals_budget":
					n = tg.Budget
				default:
					t.Fatalf("unknown core_bytes %q", v)
				}
			default:
				t.Fatalf("core_bytes is neither a number nor a keyword: %v", c.CoreBytes)
			}

			core := render.Core{
				Text:   strings.Repeat("x", n),
				SHA256: strings.Repeat("a", 64),
				Lang:   "en",
			}

			out, err := render.Render(tg, core, "1.0.0", "")

			switch c.Expect {
			case "ok":
				if err != nil {
					t.Fatalf("a core of %d bytes should render within %s's %d byte budget: %v\n%s",
						n, c.Target, tg.Budget, err, c.About)
				}
				if len(out) > tg.Budget {
					t.Errorf("rendered %d bytes, over the %d byte budget — and returned no error",
						len(out), tg.Budget)
				}
			case "error":
				if err == nil {
					t.Fatalf("a core of %d bytes rendered to %d bytes against %s's %d byte "+
						"budget with no error. Truncating or exceeding the budget silently is "+
						"the failure this rule exists to prevent.\n%s",
						n, len(out), c.Target, tg.Budget, c.About)
				}
				// Failing and leaving a partial file behind is the same defect with an error
				// message attached: the next run reads a file that is neither old nor new.
				if out != "" {
					t.Errorf("the budget was exceeded and %d bytes were still produced; "+
						"nothing must be written", len(out))
				}
			default:
				t.Fatalf("case declares no expectation")
			}
		})
	}
}
