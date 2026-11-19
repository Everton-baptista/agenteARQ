package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Baseline records the failures a project had when it adopted the standard.
//
// Without it, a project with existing agents fails dozens of controls on day one, the gate is
// switched off, and nothing improves. With it, the gate blocks only what is new or worse — so
// adoption costs nothing on the first day and the debt stays visible instead of being forgotten.
//
// This is a ratchet, not an amnesty: a baselined failure that is later fixed is removed on the
// next `--update-baseline`, and it cannot be reintroduced.
type Baseline struct {
	SchemaVersion string `json:"schema_version"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	Profile       string `json:"profile"`
	Note          string `json:"note"`
	// Accepted maps "agent\x00control" to what was failing at adoption.
	Accepted map[string]BaselineEntry `json:"accepted"`
}

// BaselineEntry is one grandfathered failure.
type BaselineEntry struct {
	Agent    string   `json:"agent"`
	Control  string   `json:"control"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message,omitempty"`
	Since    string   `json:"since"`
}

func baselineKey(agent, control string) string { return agent + "\x00" + control }

// NewBaseline records the current failures as the starting point.
func NewBaseline(results []Result, profile string, now time.Time) *Baseline {
	b := &Baseline{
		SchemaVersion: "1.0",
		CreatedAt:     now.Format("2006-01-02"),
		Profile:       profile,
		Note: "Failures present when this project adopted agentarch. The gate blocks only what " +
			"is new or worse than this. Shrink it deliberately; `agentarch score` counts what " +
			"is left. Nothing here is forgiven — it is deferred, and visible.",
		Accepted: map[string]BaselineEntry{},
	}
	for _, r := range results {
		if r.Passed || r.Skipped || r.Waived || r.Error != "" {
			continue
		}
		b.Accepted[baselineKey(r.AgentID, r.ControlID)] = BaselineEntry{
			Agent: r.AgentID, Control: r.ControlID, Severity: r.Severity,
			Message: r.Message, Since: b.CreatedAt,
		}
	}
	return b
}

// LoadBaseline reads a baseline. A missing file is not an error — most projects do not need one.
func LoadBaseline(path string) (*Baseline, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var b Baseline
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if b.Accepted == nil {
		b.Accepted = map[string]BaselineEntry{}
	}
	return &b, nil
}

// Save writes a baseline.
func (b *Baseline) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Covers reports whether a failure was present at adoption.
func (b *Baseline) Covers(r Result) bool {
	if b == nil {
		return false
	}
	entry, ok := b.Accepted[baselineKey(r.AgentID, r.ControlID)]
	if !ok {
		return false
	}
	// A control that has since become stricter is not covered. The ratchet only ever turns one
	// way: adopting a baseline must not grandfather a severity that did not exist yet.
	return sevRank(r.Severity) <= sevRank(entry.Severity)
}

// ApplyBaseline marks covered failures so the gate passes them while the score still counts them.
//
// Deliberately not the same as passing. A baselined failure is debt with a date on it, and
// hiding it entirely is how a ratchet becomes an amnesty.
func ApplyBaseline(results []Result, b *Baseline) []Result {
	if b == nil {
		return results
	}
	out := make([]Result, len(results))
	copy(out, results)
	for i := range out {
		if out[i].Passed || out[i].Skipped || out[i].Waived {
			continue
		}
		if b.Covers(out[i]) {
			out[i].Baselined = true
			out[i].BaselineSince = b.Accepted[baselineKey(out[i].AgentID, out[i].ControlID)].Since
		}
	}
	return out
}

// BaselineDrift describes how a baseline has aged against the current results.
type BaselineDrift struct {
	Fixed      []BaselineEntry // baselined and now passing — remove them
	StillOpen  []BaselineEntry
	Stale      []BaselineEntry // for agents or controls that no longer exist
	NewFailure []Result        // failing and not baselined: what the gate blocks on
}

// Diff compares a baseline with the current results.
func (b *Baseline) Diff(results []Result) BaselineDrift {
	var d BaselineDrift
	if b == nil {
		for _, r := range results {
			if !r.Passed && !r.Skipped && !r.Waived && r.Error == "" {
				d.NewFailure = append(d.NewFailure, r)
			}
		}
		return d
	}

	seen := map[string]Result{}
	for _, r := range results {
		seen[baselineKey(r.AgentID, r.ControlID)] = r
	}

	for key, entry := range b.Accepted {
		r, present := seen[key]
		switch {
		case !present:
			d.Stale = append(d.Stale, entry)
		case r.Passed || r.Skipped:
			d.Fixed = append(d.Fixed, entry)
		default:
			d.StillOpen = append(d.StillOpen, entry)
		}
	}

	for _, r := range results {
		if r.Passed || r.Skipped || r.Waived || r.Error != "" {
			continue
		}
		if !b.Covers(r) {
			d.NewFailure = append(d.NewFailure, r)
		}
	}

	byEntry := func(e []BaselineEntry) {
		sort.Slice(e, func(i, j int) bool {
			if e[i].Agent != e[j].Agent {
				return e[i].Agent < e[j].Agent
			}
			return e[i].Control < e[j].Control
		})
	}
	byEntry(d.Fixed)
	byEntry(d.StillOpen)
	byEntry(d.Stale)
	sort.Slice(d.NewFailure, func(i, j int) bool {
		return d.NewFailure[i].ControlID < d.NewFailure[j].ControlID
	})
	return d
}

// Update returns a baseline with fixed and stale entries removed.
//
// It never adds. Adding on update would let a regression enter the baseline the moment it
// appeared, which is exactly the amnesty this exists to avoid — new failures are added only by
// an explicit `--adopt`.
func (b *Baseline) Update(results []Result, now time.Time) *Baseline {
	d := b.Diff(results)
	next := &Baseline{
		SchemaVersion: b.SchemaVersion, CreatedAt: b.CreatedAt,
		UpdatedAt: now.Format("2006-01-02"), Profile: b.Profile, Note: b.Note,
		Accepted: map[string]BaselineEntry{},
	}
	for _, e := range d.StillOpen {
		next.Accepted[baselineKey(e.Agent, e.Control)] = e
	}
	return next
}
