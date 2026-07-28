package policy

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Level is a project's conformance level. The levels are cumulative and deliberately map onto
// three different kinds of claim: the agents are described, the rules block, and there is
// evidence.
type Level string

const (
	LevelNone Level = "none"
	LevelL1   Level = "L1"
	LevelL2   Level = "L2"
	LevelL3   Level = "L3"
)

// Requirement is one condition for a level, with the reason it is not met.
type Requirement struct {
	Level   Level  `json:"level"`
	ID      string `json:"id"`
	Text    string `json:"text"`
	Met     bool   `json:"met"`
	Details string `json:"details,omitempty"`
}

// Conformance is the assessed level plus everything that fed into it.
type Conformance struct {
	Level        Level         `json:"level"`
	Requirements []Requirement `json:"requirements"`
	Agents       int           `json:"agents"`
	AssessedAt   string        `json:"assessed_at"`
	// ExpiresAt is when the level must be reassessed: the earliest date at which a piece of
	// evidence goes stale. Conformance that never decays is advertising.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// AgentEvidence is what one agent contributes to the assessment.
type AgentEvidence struct {
	ID              string
	Manifest        map[string]any
	Results         []Result
	EvalCompletedAt string
	EvalMaxAgeDays  int
	EvalMetrics     []map[string]any
	RedTeamExecuted bool
	HasThreatModel  bool
	OTelEnabled     bool
	SemconvPinned   bool
	GateInCI        bool
	ShimsInSync     bool
}

// Assess determines the conformance level.
//
// Each level asks a different question. L1: is the agent described at all? L2: do the rules
// actually block? L3: is there evidence rather than assertion? A project can be thorough at L1
// and have changed nothing about its safety, which is precisely why the levels are separate and
// why the badge names which one was reached.
func Assess(agents []AgentEvidence, gateInCI, shimsInSync bool, now time.Time) Conformance {
	var reqs []Requirement
	add := func(level Level, id, text string, met bool, details string) {
		reqs = append(reqs, Requirement{Level: level, ID: id, Text: text, Met: met, Details: details})
	}

	if len(agents) == 0 {
		return Conformance{Level: LevelNone, AssessedAt: now.Format("2006-01-02"),
			Requirements: []Requirement{{Level: LevelL1, ID: "L1-AGENTS",
				Text: "at least one agent is described by a manifest", Met: false}}}
	}

	// ---- L1: the agents are described --------------------------------------
	var noOwner, noScope, noAutonomy, noStop []string
	for _, a := range agents {
		ag, _ := a.Manifest["agent"].(map[string]any)
		owner, _ := ag["owner"].(map[string]any)
		if str(owner["accountable"]) == "" {
			noOwner = append(noOwner, a.ID)
		}
		if l, _ := ag["out_of_scope"].([]any); len(l) == 0 {
			noScope = append(noScope, a.ID)
		}
		auto, _ := ag["autonomy"].(map[string]any)
		if str(auto["level"]) == "" {
			noAutonomy = append(noAutonomy, a.ID)
		}
		if sc, _ := auto["stop_conditions"].([]any); len(sc) == 0 {
			noStop = append(noStop, a.ID)
		}
	}
	add(LevelL1, "L1-OWNER", "every agent names an accountable person", len(noOwner) == 0, list(noOwner))
	add(LevelL1, "L1-SCOPE", "every agent declares what it refuses", len(noScope) == 0, list(noScope))
	add(LevelL1, "L1-AUTONOMY", "every agent declares its autonomy level", len(noAutonomy) == 0, list(noAutonomy))
	add(LevelL1, "L1-STOP", "every agent bounds its loop", len(noStop) == 0, list(noStop))
	add(LevelL1, "L1-SYNC", "the generated instruction files are in sync", shimsInSync, "")

	// ---- L2: the rules block ----------------------------------------------
	blockers := 0
	for _, a := range agents {
		blockers += len(Summarize(a.Results).Blockers)
	}
	add(LevelL2, "L2-GATE", "the gate runs in CI with fail_on: [blocker]", gateInCI, "")
	add(LevelL2, "L2-CLEAN", "no blocker-severity control is failing", blockers == 0,
		plural(blockers, "failing blocker"))

	var noGuardrails []string
	for _, a := range agents {
		ag, _ := a.Manifest["agent"].(map[string]any)
		g, _ := ag["guardrails"].(map[string]any)
		if g == nil {
			noGuardrails = append(noGuardrails, a.ID)
			continue
		}
		for _, point := range []string{"input", "output", "action"} {
			if _, present := g[point]; !present {
				noGuardrails = append(noGuardrails, a.ID+" ("+point+")")
			}
		}
	}
	add(LevelL2, "L2-GUARDRAILS", "guardrails are declared at all three points",
		len(noGuardrails) == 0, list(noGuardrails))

	// ---- L3: there is evidence --------------------------------------------
	var noEval, staleEval, noRedTeam, noThreatModel, noOTel []string
	earliestExpiry := ""

	for _, a := range agents {
		switch {
		case a.EvalCompletedAt == "":
			noEval = append(noEval, a.ID)
		default:
			d, err := time.Parse("2006-01-02", firstTen(a.EvalCompletedAt))
			maxAge := a.EvalMaxAgeDays
			if maxAge == 0 {
				maxAge = 30
			}
			if err != nil {
				staleEval = append(staleEval, a.ID+" (unparseable date)")
			} else {
				age := int(now.Sub(d).Hours() / 24)
				if age > maxAge {
					staleEval = append(staleEval, fmt.Sprintf("%s (%d days old, limit %d)", a.ID, age, maxAge))
				} else {
					// The badge expires when the first piece of evidence does. This is
					// the mechanism: an L3 badge whose evals went stale becomes an L2
					// badge on its own, without anyone choosing to downgrade it.
					exp := d.AddDate(0, 0, maxAge).Format("2006-01-02")
					if earliestExpiry == "" || exp < earliestExpiry {
						earliestExpiry = exp
					}
				}
			}
		}
		if !a.RedTeamExecuted {
			noRedTeam = append(noRedTeam, a.ID)
		}
		if !a.HasThreatModel {
			noThreatModel = append(noThreatModel, a.ID)
		}
		if !a.OTelEnabled || !a.SemconvPinned {
			noOTel = append(noOTel, a.ID)
		}
	}

	add(LevelL3, "L3-EVAL", "every agent has an evaluation result", len(noEval) == 0, list(noEval))
	add(LevelL3, "L3-FRESH", "no evaluation result is past its freshness window",
		len(staleEval) == 0, list(staleEval))
	add(LevelL3, "L3-REDTEAM", "red team has been executed", len(noRedTeam) == 0, list(noRedTeam))
	add(LevelL3, "L3-THREAT", "every agent has a threat model", len(noThreatModel) == 0, list(noThreatModel))
	add(LevelL3, "L3-OTEL", "telemetry is enabled with a pinned semconv version",
		len(noOTel) == 0, list(noOTel))

	level := LevelNone
	for _, l := range []Level{LevelL1, LevelL2, LevelL3} {
		ok := true
		for _, r := range reqs {
			if r.Level == l && !r.Met {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		level = l
	}

	c := Conformance{Level: level, Requirements: reqs, Agents: len(agents),
		AssessedAt: now.Format("2006-01-02")}
	if level == LevelL3 {
		c.ExpiresAt = earliestExpiry
	}
	return c
}

// Badge renders a shields.io endpoint document.
func (c Conformance) Badge() map[string]any {
	colour := "red"
	switch c.Level {
	case LevelL3:
		colour = "brightgreen"
	case LevelL2:
		colour = "green"
	case LevelL1:
		colour = "yellow"
	}
	msg := string(c.Level)
	if c.Level == LevelNone {
		msg = "not conformant"
	}
	return map[string]any{
		"schemaVersion": 1,
		"label":         "agentarch",
		"message":       msg,
		"color":         colour,
	}
}

// Dimension is one axis of the maturity score.
type Dimension struct {
	Name string `json:"name"`
	// Declared is the share of controls satisfied by a manifest field — someone wrote
	// something down. Proven is the share resting on an artifact that had to be produced.
	// Reporting them together is how "we are compliant" comes to mean nothing.
	Declared float64 `json:"declared"`
	Proven   float64 `json:"proven"`
	Total    int     `json:"total"`
}

// Score computes maturity by control type. It never blocks — a number that gates a release
// becomes a number people optimise.
func Score(results []Result) []Dimension {
	type acc struct{ declTotal, declPass, provTotal, provPass int }
	byType := map[string]*acc{}

	for _, r := range results {
		if r.Skipped {
			continue
		}
		parts := strings.Split(r.ControlID, ".")
		if len(parts) < 4 {
			continue
		}
		t := parts[2]
		if byType[t] == nil {
			byType[t] = &acc{}
		}
		a := byType[t]

		proven := false
		for _, e := range r.Evidence {
			if e != "manifest_field" {
				proven = true
				break
			}
		}
		if proven {
			a.provTotal++
			if r.Passed {
				a.provPass++
			}
		} else {
			a.declTotal++
			if r.Passed {
				a.declPass++
			}
		}
	}

	var out []Dimension
	for t, a := range byType {
		d := Dimension{Name: t, Total: a.declTotal + a.provTotal}
		if a.declTotal > 0 {
			d.Declared = float64(a.declPass) / float64(a.declTotal) * 100
		}
		if a.provTotal > 0 {
			d.Proven = float64(a.provPass) / float64(a.provTotal) * 100
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func list(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	if len(xs) > 4 {
		return strings.Join(xs[:4], ", ") + fmt.Sprintf(" and %d more", len(xs)-4)
	}
	return strings.Join(xs, ", ")
}

func plural(n int, noun string) string {
	if n == 0 {
		return ""
	}
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func firstTen(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
