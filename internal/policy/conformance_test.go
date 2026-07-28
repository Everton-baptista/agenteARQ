package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
	"gopkg.in/yaml.v3"
)

type conformanceFile struct {
	Now   string `yaml:"now"`
	Cases []struct {
		ID     string `yaml:"id"`
		Expr   string `yaml:"expr"`
		Ctx    any    `yaml:"ctx"`
		Result *bool  `yaml:"result"`
		Error  string `yaml:"error"`
	} `yaml:"cases"`
}

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

// TestSpecConformance runs the fixtures in spec/conformance/expr against this implementation.
//
// The point of the file is that it is not ours: any implementation claiming spec/1.0 runs the
// same cases. Keeping the reference implementation honest against it is what stops the spec
// from quietly becoming "whatever this Go code happens to do".
func TestSpecConformance(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "spec", "conformance", "expr", "cases.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f conformanceFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("conformance suite is empty")
	}

	now, err := time.Parse("2006-01-02", f.Now)
	if err != nil {
		t.Fatalf("fixture `now` is not a date: %v", err)
	}

	for _, c := range f.Cases {
		t.Run(c.ID, func(t *testing.T) {
			if c.Result == nil && c.Error == "" {
				t.Fatal("case declares neither an expected result nor an expected error")
			}

			// Round-trip through JSON so the context holds exactly the generic types the
			// spec defines, rather than yaml.v3's map[any]any.
			b, err := json.Marshal(c.Ctx)
			if err != nil {
				t.Fatal(err)
			}
			var ctx map[string]any
			if err := json.Unmarshal(b, &ctx); err != nil {
				ctx = map[string]any{}
			}

			got, evalErr := policy.EvalString(c.Expr, ctx, now)

			if c.Error != "" {
				if evalErr == nil {
					t.Fatalf("%q must be rejected, but it evaluated to %v", c.Expr, got)
				}
				// `any` asserts only that the expression was rejected. Pinning exact
				// wording would force every implementation to copy this one's phrasing.
				if c.Error != "any" && !strings.Contains(evalErr.Error(), c.Error) {
					t.Fatalf("error should mention %q; got: %v", c.Error, evalErr)
				}
				return
			}

			if evalErr != nil {
				t.Fatalf("unexpected error for %q: %v", c.Expr, evalErr)
			}
			if got != *c.Result {
				t.Fatalf("%q = %v, want %v", c.Expr, got, *c.Result)
			}
		})
	}
}
