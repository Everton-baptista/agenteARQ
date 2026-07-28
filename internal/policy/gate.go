package policy

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Severity ordering. Resolution takes the highest when two packs disagree, because the safe
// reading of a conflict is the strict one.
type Severity string

const (
	SevBlocker Severity = "blocker"
	SevMajor   Severity = "major"
	SevMinor   Severity = "minor"
	SevWarn    Severity = "warn" // a control inside its grace period
)

func sevRank(s Severity) int {
	switch s {
	case SevBlocker:
		return 3
	case SevMajor:
		return 2
	case SevMinor:
		return 1
	}
	return 0
}

// Control is one rule, loaded from content/packs/controls/.
type Control struct {
	ID          string   `yaml:"id"`
	Type        string   `yaml:"type"`
	Title       string   `yaml:"title"`
	Intent      string   `yaml:"intent"`
	Status      string   `yaml:"status"`
	AppliesTo   Applies  `yaml:"applies_to"`
	Check       Check    `yaml:"check"`
	Evidence    []string `yaml:"evidence"`
	Remediation string   `yaml:"remediation"`
	StandardRef string   `yaml:"standard_ref"`
	References  []string `yaml:"references"`
}

type Applies struct {
	SystemType            []string `yaml:"system_type"`
	AutonomyMin           string   `yaml:"autonomy_min"`
	StageMin              string   `yaml:"stage_min"`
	ProcessesPersonalData *bool    `yaml:"processes_personal_data"`
}

type Check struct {
	Kind     string `yaml:"kind"`
	Expr     string `yaml:"expr"`
	PathExpr string `yaml:"path_expr"`
	Message  string `yaml:"message"`
}

// Pack is a versioned set of control requirements.
type Pack struct {
	ID              string      `yaml:"id"`
	Version         string      `yaml:"version"`
	Title           string      `yaml:"title"`
	Description     string      `yaml:"description"`
	AuthorityStatus string      `yaml:"authority_status"`
	Authority       Authority   `yaml:"authority"`
	ReviewedAt      string      `yaml:"reviewed_at"`
	AppliesWhen     AppliesWhen `yaml:"applies_when"`
	Requires        []Require   `yaml:"requires"`
}

type Authority struct {
	Name         string   `yaml:"name"`
	Reference    string   `yaml:"reference"`
	Jurisdiction []string `yaml:"jurisdiction"`
	URL          string   `yaml:"url"`
}

type AppliesWhen struct {
	SystemType            []string `yaml:"system_type"`
	Stage                 []string `yaml:"stage"`
	ProcessesPersonalData *bool    `yaml:"processes_personal_data"`
	Audience              []string `yaml:"audience"`
	Jurisdictions         []string `yaml:"jurisdictions"`
}

type Require struct {
	Control          string   `yaml:"control"`
	Severity         Severity `yaml:"severity"`
	EnforcedFrom     string   `yaml:"enforced_from"`
	RequiredEvidence []string `yaml:"required_evidence"`
	Note             string   `yaml:"note"`
}

// Catalog is everything loaded from a content tree.
type Catalog struct {
	Controls map[string]Control
	Packs    map[string]Pack
}

// LoadCatalog reads controls and packs out of a content tree (embedded payload or an installed
// agentarch/std — both are rooted the same way).
func LoadCatalog(fsys fs.FS) (*Catalog, error) {
	cat := &Catalog{Controls: map[string]Control{}, Packs: map[string]Pack{}}

	err := fs.WalkDir(fsys, "packs", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return rerr
		}
		switch {
		case strings.Contains(p, "packs/controls/") && strings.HasSuffix(p, ".yaml"):
			var doc struct {
				Control Control `yaml:"control"`
			}
			if e := yaml.Unmarshal(raw, &doc); e != nil {
				return fmt.Errorf("%s: %w", p, e)
			}
			if doc.Control.ID == "" {
				return fmt.Errorf("%s: control has no id", p)
			}
			if prev, dup := cat.Controls[doc.Control.ID]; dup {
				return fmt.Errorf("control %s defined twice (%s)", prev.ID, p)
			}
			cat.Controls[doc.Control.ID] = doc.Control
		case filepath.Base(p) == "pack.yaml":
			var doc struct {
				Pack Pack `yaml:"pack"`
			}
			if e := yaml.Unmarshal(raw, &doc); e != nil {
				return fmt.Errorf("%s: %w", p, e)
			}
			if doc.Pack.ID == "" {
				return fmt.Errorf("%s: pack has no id", p)
			}
			cat.Packs[doc.Pack.ID] = doc.Pack
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cat, nil
}

// Profiles are named pack selections. Kept in code rather than data because they are part of
// the CLI's contract with the user, not something a third-party pack should be able to redefine.
var Profiles = map[string][]string{
	"minimal":   {"core.agent"},
	"standard":  {"core.agent", "sec.owasp-llm"},
	"regulated": {"core.agent", "sec.owasp-llm", "obs.otel", "eval.baseline"},
}

// Waiver is a time-boxed, owned exception.
type Waiver struct {
	Control string `yaml:"control"`
	Agent   string `yaml:"agent"`
	Reason  string `yaml:"reason"`
	Owner   string `yaml:"owner"`
	Until   string `yaml:"until"`
}

// LoadWaivers reads project/waivers.yaml. A missing file is not an error.
func LoadWaivers(path string) ([]Waiver, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Waivers []Waiver `yaml:"waivers"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("waivers.yaml: %w", err)
	}
	return doc.Waivers, nil
}

// MaxWaiverDays bounds how long an exception may live. An exception with no end date is not an
// exception, it is a quiet amendment to the standard.
const MaxWaiverDays = 90

// Result is one control's outcome for one agent.
type Result struct {
	AgentID     string   `json:"agent"`
	ControlID   string   `json:"control"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Passed      bool     `json:"passed"`
	Skipped     bool     `json:"skipped,omitempty"`
	SkipReason  string   `json:"skip_reason,omitempty"`
	Waived      bool     `json:"waived,omitempty"`
	WaiverUntil string   `json:"waiver_until,omitempty"`
	WaiverOwner string   `json:"waiver_owner,omitempty"`
	Message     string   `json:"message,omitempty"`
	Remediation string   `json:"remediation,omitempty"`
	FromPack    string   `json:"from_pack"`
	PackVersion string   `json:"pack_version"`
	Evidence    []string `json:"evidence,omitempty"`
	Error       string   `json:"error,omitempty"`
	StandardRef string   `json:"standard_ref,omitempty"`
}

// Resolution records which pack imposed a control at which severity, so `--explain-resolution`
// can answer "why am I being blocked by this" without anyone reading the validator's source.
type Resolution struct {
	ControlID  string
	Severity   Severity
	FromPack   string
	Version    string
	Superseded []string
}

// ResolvePacks selects the applicable packs for an agent and merges their requirements.
//
// Where two packs require the same control at different severities the highest wins. A conflict
// resolved downward would let an organisation pack quietly weaken a security pack it imported.
func ResolvePacks(cat *Catalog, packIDs []string, agent map[string]any) ([]Resolution, []string, error) {
	byControl := map[string]Resolution{}
	var missing []string

	for _, id := range packIDs {
		p, ok := cat.Packs[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		if !packApplies(p, agent) {
			continue
		}
		for _, r := range p.Requires {
			sev := r.Severity
			// A control inside its grace period runs in warn mode. No control is born
			// blocking: a rule that starts failing the day it ships gets the whole gate
			// switched off rather than getting anything fixed.
			if r.EnforcedFrom != "" && semverLess(p.Version, r.EnforcedFrom) {
				sev = SevWarn
			}
			cur, seen := byControl[r.Control]
			if !seen {
				byControl[r.Control] = Resolution{ControlID: r.Control, Severity: sev, FromPack: p.ID, Version: p.Version}
				continue
			}
			if sevRank(sev) > sevRank(cur.Severity) {
				cur.Superseded = append(cur.Superseded, fmt.Sprintf("%s@%s (%s)", cur.FromPack, cur.Version, cur.Severity))
				cur.Severity, cur.FromPack, cur.Version = sev, p.ID, p.Version
			} else {
				cur.Superseded = append(cur.Superseded, fmt.Sprintf("%s@%s (%s)", p.ID, p.Version, sev))
			}
			byControl[r.Control] = cur
		}
	}

	out := make([]Resolution, 0, len(byControl))
	for _, r := range byControl {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ControlID < out[j].ControlID })
	return out, missing, nil
}

func packApplies(p Pack, agent map[string]any) bool {
	w := p.AppliesWhen
	if len(w.SystemType) > 0 && !containsStr(w.SystemType, str(agent["system_type"])) {
		return false
	}
	if len(w.Stage) > 0 && !containsStr(w.Stage, str(agent["stage"])) {
		return false
	}
	if len(w.Audience) > 0 {
		users, _ := agent["users"].(map[string]any)
		if !containsStr(w.Audience, str(users["audience"])) {
			return false
		}
	}
	if w.ProcessesPersonalData != nil {
		priv, _ := agent["privacy"].(map[string]any)
		got, _ := priv["processes_personal_data"].(bool)
		if got != *w.ProcessesPersonalData {
			return false
		}
	}
	// Jurisdiction packs apply only where the agent says it operates. This is what keeps a
	// regional obligation from being imposed on a team it does not apply to — which is the
	// fastest way to make an international standard un-adoptable.
	if len(w.Jurisdictions) > 0 {
		declared, _ := agent["jurisdictions"].([]any)
		hit := false
		for _, d := range declared {
			if containsStr(w.Jurisdictions, str(d)) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// controlApplies narrows an individual control within an applicable pack.
func controlApplies(c Control, agent map[string]any) (bool, string) {
	a := c.AppliesTo
	if len(a.SystemType) > 0 && !containsStr(a.SystemType, str(agent["system_type"])) {
		return false, "system_type does not match"
	}
	if a.AutonomyMin != "" {
		auto, _ := agent["autonomy"].(map[string]any)
		if autonomyRank(str(auto["level"])) < autonomyRank(a.AutonomyMin) {
			return false, "autonomy below " + a.AutonomyMin
		}
	}
	if a.StageMin != "" && stageRank(str(agent["stage"])) < stageRank(a.StageMin) {
		return false, "stage below " + a.StageMin
	}
	if a.ProcessesPersonalData != nil {
		priv, _ := agent["privacy"].(map[string]any)
		got, _ := priv["processes_personal_data"].(bool)
		if got != *a.ProcessesPersonalData {
			return false, "personal-data condition not met"
		}
	}
	return true, ""
}

func autonomyRank(l string) int {
	for i, v := range []string{"L0_suggest", "L1_act_with_approval", "L2_act_reversible",
		"L3_act_irreversible_bounded", "L4_autonomous"} {
		if v == l {
			return i
		}
	}
	return -1
}

func stageRank(s string) int {
	for i, v := range []string{"prototype", "internal", "pilot", "production"} {
		if v == s {
			return i
		}
	}
	return -1
}

// Evaluate runs the resolved controls against one agent's context.
func Evaluate(cat *Catalog, res []Resolution, ctx map[string]any, waivers []Waiver, now time.Time) []Result {
	agent, _ := ctx["agent"].(map[string]any)
	agentID := str(agent["id"])
	var out []Result

	for _, r := range res {
		c, ok := cat.Controls[r.ControlID]
		if !ok {
			out = append(out, Result{
				AgentID: agentID, ControlID: r.ControlID, Severity: r.Severity,
				FromPack: r.FromPack, PackVersion: r.Version,
				Error: "control is required by the pack but not defined in the catalogue",
			})
			continue
		}

		base := Result{
			AgentID: agentID, ControlID: c.ID, Title: c.Title, Severity: r.Severity,
			FromPack: r.FromPack, PackVersion: r.Version, Evidence: c.Evidence,
			Remediation: c.Remediation, StandardRef: c.StandardRef,
		}

		if ok, why := controlApplies(c, agent); !ok {
			base.Skipped, base.SkipReason, base.Passed = true, why, true
			out = append(out, base)
			continue
		}

		switch c.Check.Kind {
		case "manual_attestation":
			// Honest about what it is: nobody automated this, a human asserted it.
			base.Skipped = true
			base.SkipReason = "manual attestation — not machine verifiable"
			base.Passed = true
		case "static_manifest", "eval_threshold":
			pass, err := EvalString(c.Check.Expr, ctx, now)
			if err != nil {
				// A broken expression is an error, never a silent pass.
				base.Error = err.Error()
				base.Passed = false
			} else {
				base.Passed = pass
				if !pass {
					base.Message = c.Check.Message
				}
			}
		case "file_exists":
			base.Passed = true // resolved by the caller, which knows the agent directory
		default:
			base.Error = "unknown check kind " + c.Check.Kind
		}

		if !base.Passed {
			if w, found := activeWaiver(waivers, c.ID, agentID, now); found {
				base.Waived = true
				base.WaiverUntil = w.Until
				base.WaiverOwner = w.Owner
			}
		}
		out = append(out, base)
	}
	return out
}

// activeWaiver finds an unexpired waiver. An expired one deliberately does not suppress the
// finding — expiry is the whole mechanism.
func activeWaiver(ws []Waiver, controlID, agentID string, now time.Time) (Waiver, bool) {
	for _, w := range ws {
		if w.Control != controlID {
			continue
		}
		if w.Agent != "" && w.Agent != agentID {
			continue
		}
		until, err := time.Parse("2006-01-02", w.Until)
		if err != nil || !until.After(now) {
			continue
		}
		return w, true
	}
	return Waiver{}, false
}

// WaiverProblem is a waiver that cannot be honoured.
type WaiverProblem struct {
	Waiver Waiver
	Reason string
}

// CheckWaivers validates the waiver file itself. These produce a distinct exit code from a
// blocked gate so that "your exception lapsed" reaches its owner instead of alarming everyone.
func CheckWaivers(ws []Waiver, now time.Time) []WaiverProblem {
	var out []WaiverProblem
	for _, w := range ws {
		switch {
		case w.Control == "":
			out = append(out, WaiverProblem{w, "no control id"})
		case strings.TrimSpace(w.Owner) == "":
			out = append(out, WaiverProblem{w, "no owner — an unowned exception is nobody's problem to close"})
		case strings.TrimSpace(w.Reason) == "":
			out = append(out, WaiverProblem{w, "no reason recorded"})
		case w.Until == "":
			out = append(out, WaiverProblem{w, "no expiry — an exception with no end date is a quiet amendment to the standard"})
		default:
			until, err := time.Parse("2006-01-02", w.Until)
			switch {
			case err != nil:
				out = append(out, WaiverProblem{w, "expiry is not a YYYY-MM-DD date"})
			case !until.After(now):
				out = append(out, WaiverProblem{w, fmt.Sprintf("expired on %s", w.Until)})
			case until.Sub(now) > MaxWaiverDays*24*time.Hour:
				out = append(out, WaiverProblem{w,
					fmt.Sprintf("expires %s, more than %d days away", w.Until, MaxWaiverDays)})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- helpers

func str(v any) string {
	s, _ := v.(string)
	return s
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s || x == "*" {
			return true
		}
	}
	return false
}

// semverLess compares dotted versions numerically. Used only for grace periods, where a
// mis-parse should not silently enforce something early — hence the conservative default.
func semverLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscanf(pa[i], "%d", &x)
		}
		if i < len(pb) {
			fmt.Sscanf(pb[i], "%d", &y)
		}
		if x != y {
			return x < y
		}
	}
	return false
}
