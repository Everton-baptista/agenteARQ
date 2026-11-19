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

// ---------------------------------------------------------------- resolution

// resolutionFile mirrors spec/conformance/resolution/cases.yaml. Each case carries its own
// catalogue, so a fixture never depends on the content an implementation happens to ship.
type resolutionFile struct {
	Cases []struct {
		ID        string `yaml:"id"`
		About     string `yaml:"about"`
		Catalogue struct {
			Controls []struct {
				ID string `yaml:"id"`
			} `yaml:"controls"`
			Packs []struct {
				ID              string `yaml:"id"`
				Version         string `yaml:"version"`
				AuthorityStatus string `yaml:"authority_status"`
				AppliesWhen     struct {
					SystemType            []string `yaml:"system_type"`
					Stage                 []string `yaml:"stage"`
					Audience              []string `yaml:"audience"`
					Jurisdictions         []string `yaml:"jurisdictions"`
					ProcessesPersonalData *bool    `yaml:"processes_personal_data"`
				} `yaml:"applies_when"`
				Requires []struct {
					Control      string `yaml:"control"`
					Severity     string `yaml:"severity"`
					EnforcedFrom string `yaml:"enforced_from"`
				} `yaml:"requires"`
			} `yaml:"packs"`
		} `yaml:"catalogue"`
		ProfilePacks []string       `yaml:"profile_packs"`
		Agent        map[string]any `yaml:"agent"`
		Expect       struct {
			Resolved map[string]string `yaml:"resolved"`
			Missing  []string          `yaml:"missing"`
		} `yaml:"expect"`
	} `yaml:"cases"`
}

// TestSpecConformanceResolution runs spec/conformance/resolution against this implementation.
//
// Resolution is where a second implementation can be wrong without anyone noticing: everything
// still runs, and the answer is quietly weaker. A binding-law floor that resolves downward, or a
// grace period applied after the severity merge instead of before, produces a gate that passes
// and should not have.
func TestSpecConformanceResolution(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "spec", "conformance", "resolution", "cases.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f resolutionFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("resolution conformance suite is empty")
	}

	for _, c := range f.Cases {
		t.Run(c.ID, func(t *testing.T) {
			cat := &policy.Catalog{
				Controls: map[string]policy.Control{},
				Packs:    map[string]policy.Pack{},
			}
			for _, ctl := range c.Catalogue.Controls {
				cat.Controls[ctl.ID] = policy.Control{ID: ctl.ID, Status: "stable"}
			}
			for _, p := range c.Catalogue.Packs {
				pack := policy.Pack{
					ID:              p.ID,
					Version:         p.Version,
					AuthorityStatus: p.AuthorityStatus,
					AppliesWhen: policy.AppliesWhen{
						SystemType:            p.AppliesWhen.SystemType,
						Stage:                 p.AppliesWhen.Stage,
						Audience:              p.AppliesWhen.Audience,
						Jurisdictions:         p.AppliesWhen.Jurisdictions,
						ProcessesPersonalData: p.AppliesWhen.ProcessesPersonalData,
					},
				}
				for _, r := range p.Requires {
					pack.Requires = append(pack.Requires, policy.Require{
						Control:      r.Control,
						Severity:     policy.Severity(r.Severity),
						EnforcedFrom: r.EnforcedFrom,
					})
				}
				cat.Packs[p.ID] = pack
			}

			// Round-trip so the agent holds the generic types the spec defines rather than
			// yaml.v3's map[any]any.
			b, err := json.Marshal(c.Agent)
			if err != nil {
				t.Fatal(err)
			}
			var agent map[string]any
			if err := json.Unmarshal(b, &agent); err != nil {
				t.Fatal(err)
			}

			// §1: the selected set is the union of the profile's packs and the manifest's.
			packIDs := append([]string{}, c.ProfilePacks...)
			if pol, ok := agent["policy"].(map[string]any); ok {
				if list, ok := pol["packs"].([]any); ok {
					for _, v := range list {
						if s, ok := v.(string); ok {
							packIDs = append(packIDs, s)
						}
					}
				}
			}

			res, missing, err := policy.ResolvePacks(cat, packIDs, agent)
			if err != nil {
				t.Fatalf("resolution failed: %v", err)
			}

			if len(c.Expect.Missing) > 0 {
				if len(missing) != len(c.Expect.Missing) {
					t.Errorf("missing packs = %v, want %v", missing, c.Expect.Missing)
				}
				for i, want := range c.Expect.Missing {
					if i < len(missing) && missing[i] != want {
						t.Errorf("missing[%d] = %q, want %q", i, missing[i], want)
					}
				}
			}

			// Order is deliberately not asserted: §5 says order MUST NOT affect results, so
			// pinning one would make a parallel implementation non-conforming for no reason.
			got := map[string]string{}
			for _, r := range res {
				got[r.ControlID] = string(r.Severity)
			}

			for id, want := range c.Expect.Resolved {
				if got[id] != want {
					t.Errorf("%s resolved to %q, want %q\n%s", id, got[id], want, c.About)
				}
			}
			for id, sev := range got {
				if _, ok := c.Expect.Resolved[id]; !ok {
					t.Errorf("%s resolved to %q and should not have been selected at all\n%s",
						id, sev, c.About)
				}
			}
		})
	}
}

// ---------------------------------------------------------------- exit codes

type exitCodeFile struct {
	Codes      map[int]string `yaml:"codes"`
	Precedence []string       `yaml:"precedence"`
	Cases      []struct {
		ID         string   `yaml:"id"`
		About      string   `yaml:"about"`
		Conditions []string `yaml:"conditions"`
		ExitCode   int      `yaml:"exit_code"`
	} `yaml:"cases"`
}

// TestSpecConformanceExitCodes runs spec/conformance/exit-codes against this implementation.
//
// Exit codes are the whole machine-readable surface of the tool — nothing else it prints is
// versioned. The part worth pinning is the precedence, because each code looks obviously right
// on its own and the ordering between them is the thing a second implementation invents.
func TestSpecConformanceExitCodes(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "spec", "conformance", "exit-codes", "cases.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f exitCodeFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) == 0 {
		t.Fatal("exit-code conformance suite is empty")
	}

	// The codes the fixture declares must be the codes the implementation uses. A suite that
	// agreed with itself and not with the binary would pin nothing.
	for code, meaning := range map[int]int{
		policy.ExitOK: 0, policy.ExitUsage: 1, policy.ExitStructure: 2,
		policy.ExitDrift: 3, policy.ExitGate: 4, policy.ExitWaiver: 5,
		policy.ExitRevalidated: 6,
	} {
		if code != meaning {
			t.Errorf("exit code constant is %d, the spec says %d", code, meaning)
		}
		if _, ok := f.Codes[meaning]; !ok {
			t.Errorf("the fixture does not document exit code %d", meaning)
		}
	}

	for _, c := range f.Cases {
		t.Run(c.ID, func(t *testing.T) {
			conds := make([]policy.Condition, 0, len(c.Conditions))
			for _, s := range c.Conditions {
				conds = append(conds, policy.Condition(s))
			}
			if got := policy.ExitCode(conds...); got != c.ExitCode {
				t.Errorf("conditions %v produced exit %d, want %d\n%s",
					c.Conditions, got, c.ExitCode, c.About)
			}
		})
	}
}

// An unknown condition must not silently resolve to success. A fixture naming a condition the
// implementation does not know would otherwise pass as "nothing wrong".
func TestUnknownConditionDoesNotReportSuccessSilently(t *testing.T) {
	if got := policy.ExitCode("not-a-condition"); got != policy.ExitOK {
		t.Fatalf("unknown condition produced exit %d", got)
	}
	// It reports 0, which is correct — but only because nothing else held. Paired with a real
	// condition, the real one must still win.
	if got := policy.ExitCode("not-a-condition", policy.CondBlocker); got != policy.ExitGate {
		t.Errorf("an unknown condition suppressed a real one: exit %d, want %d",
			got, policy.ExitGate)
	}
}
