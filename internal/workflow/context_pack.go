package workflow

import (
	"os"
	"strings"
)

// The assistant should not re-read the whole specification to learn what one task must do: that
// costs tokens on every step and grows with the product. The context pack is the slice of the
// contract one task needs, computed here, bounded, and the same for every host.

// ContextPackLimit bounds the pack. About 1.5k tokens, the size Anthropic recommends for a
// subagent's summary and the size of a default repository map.
const ContextPackLimit = 6000

const contextTruncated = "… [truncated; read " + "agentarch/project/spec.md for the rest]"

type TaskContext struct {
	TaskID     string     `json:"task_id"`
	Title      string     `json:"title"`
	Criteria   []string   `json:"criteria"`
	Scenarios  []Scenario `json:"scenarios,omitempty"`
	Checks     []string   `json:"checks"`
	DependsOn  []string   `json:"depends_on,omitempty"`
	Truncated  bool       `json:"truncated,omitempty"`
	LimitBytes int        `json:"limit_bytes"`
}

// criterionText returns the paragraph of spec.md that names the criterion: a list item, a line,
// or the paragraph it sits in.
func criterionText(spec, id string) string {
	for _, para := range strings.Split(spec, "\n\n") {
		for _, line := range strings.Split(para, "\n") {
			if criterionInText.MatchString(line) && containsWord(line, id) {
				// A list item is self-contained; a prose paragraph is kept whole.
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "#") {
					return trimmed
				}
				return strings.TrimSpace(para)
			}
		}
	}
	return id
}

func containsWord(line, id string) bool {
	for _, m := range criterionInText.FindAllString(line, -1) {
		if m == id {
			return true
		}
	}
	return false
}

func (s *Service) taskContext(t Task) *TaskContext {
	pack := &TaskContext{TaskID: t.ID, Title: t.Title, Checks: t.Checks, DependsOn: t.DependsOn, LimitBytes: ContextPackLimit}
	specPath, _ := safePath(s.Root, documents[1])
	spec, _ := os.ReadFile(specPath)
	budget := ContextPackLimit - len(t.Title) - len(t.ID)
	for _, c := range t.Checks {
		budget -= len(c)
	}
	add := func(text string) bool {
		if len(text) > budget {
			if budget > len(contextTruncated)+40 {
				pack.Criteria = append(pack.Criteria, truncateUTF8(text, budget-len(contextTruncated))+contextTruncated)
			}
			pack.Truncated = true
			budget = 0
			return false
		}
		budget -= len(text)
		return true
	}
	for _, a := range t.Acceptance {
		text := criterionText(string(spec), a)
		if !add(text) {
			break
		}
		pack.Criteria = append(pack.Criteria, text)
	}
	if s.exists(ScenariosPath) && !pack.Truncated {
		if d, e := s.loadScenarios(); e == nil {
			for _, sc := range d.Scenarios {
				if !sharesCriterion(sc.Acceptance, t.Acceptance) {
					continue
				}
				size := len(sc.ID) + len(sc.Given) + len(sc.When) + len(sc.Then) + 40
				if size > budget {
					pack.Truncated = true
					break
				}
				budget -= size
				pack.Scenarios = append(pack.Scenarios, sc)
			}
		}
	}
	return pack
}

func sharesCriterion(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}

// truncateUTF8 cuts at a rune boundary at or below n bytes.
func truncateUTF8(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}
