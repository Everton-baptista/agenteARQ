package emit

import (
	"fmt"
	"html"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

// Report is everything a report renders.
type Report struct {
	Project     string
	Profile     string
	GeneratedAt time.Time
	Version     string
	Results     []policy.Result
	Summary     policy.Summary
	Conformance policy.Conformance
	Dimensions  []policy.Dimension
	Waivers     []policy.Waiver
	WaiverIssue []policy.WaiverProblem
}

// byAgent groups results, preserving a stable order.
func (r Report) byAgent() ([]string, map[string][]policy.Result) {
	m := map[string][]policy.Result{}
	for _, res := range r.Results {
		m[res.AgentID] = append(m[res.AgentID], res)
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
		sort.Slice(m[id], func(i, j int) bool { return m[id][i].ControlID < m[id][j].ControlID })
	}
	sort.Strings(ids)
	return ids, m
}

// Markdown writes a report meant to be read in a pull request or pasted into a document.
//
// It leads with what is failing rather than with a score. A report that opens with a percentage
// invites the reader to treat the number as the finding, and the number is the least actionable
// thing in it.
func Markdown(w io.Writer, r Report) error {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("# agentarch report\n\n")
	p("%s · profile `%s` · generated %s by agentarch %s\n\n",
		r.Project, r.Profile, r.GeneratedAt.Format("2006-01-02"), r.Version)

	// What is wrong, first.
	switch {
	case len(r.Summary.Blockers) > 0:
		p("## Blocked\n\n%d blocker-severity control(s) are failing. A release is not ready.\n\n",
			len(r.Summary.Blockers))
		p("| Agent | Control | What is wrong | Fix |\n|---|---|---|---|\n")
		for _, res := range r.Summary.Blockers {
			p("| `%s` | `%s` | %s | %s |\n",
				res.AgentID, res.ControlID, oneLine(res.Message), oneLine(res.Remediation))
		}
		p("\n")
	case len(r.Summary.Errors) > 0:
		p("## Could not be evaluated\n\n")
		p("%d control(s) failed to evaluate. This is a defect in the pack or the artifacts, "+
			"not a finding about the agent — an unevaluated control is not coverage.\n\n",
			len(r.Summary.Errors))
	default:
		p("## Not blocked\n\nNo blocker-severity control is failing.\n\n")
	}

	if len(r.WaiverIssue) > 0 {
		p("## Waivers needing attention\n\n")
		for _, wp := range r.WaiverIssue {
			p("- `%s` (%s) — %s. Owner: %s\n",
				wp.Waiver.Control, orAll(wp.Waiver.Agent), wp.Reason, wp.Waiver.Owner)
		}
		p("\nAn expired waiver does not quietly stop applying; it surfaces so the person who " +
			"took the decision hears about it.\n\n")
	}

	p("## Conformance\n\n**%s**", r.Conformance.Level)
	if r.Conformance.ExpiresAt != "" {
		p(" — valid until %s, after which it drops to L2 on its own. "+
			"Conformance that never decays is advertising.", r.Conformance.ExpiresAt)
	}
	p("\n\n| Level | Requirement | |\n|---|---|---|\n")
	for _, req := range r.Conformance.Requirements {
		mark := "✓"
		detail := ""
		if !req.Met {
			mark = "✗"
			detail = req.Details
		}
		p("| %s | %s | %s %s |\n", req.Level, req.Text, mark, detail)
	}
	p("\n")

	if len(r.Dimensions) > 0 {
		p("## Maturity\n\n")
		p("`declared` is satisfied by a field someone wrote down. `proven` rests on an artifact " +
			"that had to be produced. They are kept apart because collapsing them is how " +
			"\"we are compliant\" comes to mean nothing.\n\n")
		p("| Dimension | Declared | Proven | Controls |\n|---|---:|---:|---:|\n")
		for _, d := range r.Dimensions {
			proven := fmt.Sprintf("%.0f%%", d.Proven)
			if d.Proven == 0 && d.Declared > 0 {
				proven = "—"
			}
			p("| %s | %.0f%% | %s | %d |\n", d.Name, d.Declared, proven, d.Total)
		}
		p("\n")
	}

	ids, byAgent := r.byAgent()
	p("## By agent\n\n")
	for _, id := range ids {
		res := byAgent[id]
		var failing []policy.Result
		for _, x := range res {
			if !x.Passed && !x.Skipped {
				failing = append(failing, x)
			}
		}
		p("### `%s`\n\n%d control(s) evaluated, %d finding(s).\n\n", id, len(res), len(failing))
		if len(failing) == 0 {
			p("Nothing outstanding.\n\n")
			continue
		}
		p("| Severity | Control | State | Detail |\n|---|---|---|---|\n")
		for _, x := range failing {
			state := "failing"
			switch {
			case x.Waived:
				state = "waived until " + x.WaiverUntil
			case x.Baselined:
				state = "baselined " + x.BaselineSince
			}
			p("| %s | `%s` | %s | %s |\n",
				x.Severity, x.ControlID, state, oneLine(x.Message))
		}
		p("\n")
	}

	p("---\n\nRun `agentarch explain <control.id>` for the reasoning behind any control, and " +
		"what changing it would take.\n")
	return nil
}

// HTML writes a self-contained page. No external requests: a report is often read from a
// filesystem, an artifact store or an air-gapped machine.
func HTML(w io.Writer, r Report) error {
	md := &strings.Builder{}
	if err := Markdown(md, r); err != nil {
		return err
	}

	status, colour := "not blocked", "#1a7f37"
	if len(r.Summary.Blockers) > 0 {
		status, colour = fmt.Sprintf("%d blocker(s)", len(r.Summary.Blockers)), "#cf222e"
	}

	fmt.Fprintf(w, `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>agentarch report — %s</title>
<style>
  :root { color-scheme: light dark; --fg:#1f2328; --bg:#fff; --muted:#59636e; --line:#d1d9e0; }
  @media (prefers-color-scheme: dark) {
    :root { --fg:#e6edf3; --bg:#0d1117; --muted:#9198a1; --line:#3d444d; }
  }
  body { font: 15px/1.6 ui-sans-serif, system-ui, sans-serif; color: var(--fg);
         background: var(--bg); max-width: 60rem; margin: 0 auto; padding: 2rem 1.25rem 4rem; }
  h1 { font-size: 1.6rem; margin-bottom: .25rem; }
  h2 { font-size: 1.15rem; margin-top: 2.5rem; padding-bottom: .3rem;
       border-bottom: 1px solid var(--line); }
  h3 { font-size: 1rem; margin-top: 1.75rem; }
  table { border-collapse: collapse; width: 100%%; margin: 1rem 0; font-size: .9rem;
          display: block; overflow-x: auto; }
  th, td { text-align: left; padding: .5rem .6rem; border-bottom: 1px solid var(--line);
           vertical-align: top; }
  th { font-weight: 600; color: var(--muted); }
  code { font: .87em ui-monospace, SFMono-Regular, monospace;
         background: color-mix(in srgb, var(--fg) 8%%, transparent);
         padding: .1em .35em; border-radius: 4px; }
  .status { display: inline-block; padding: .3rem .7rem; border-radius: 999px;
            background: %s; color: #fff; font-weight: 600; font-size: .85rem; }
  .meta { color: var(--muted); font-size: .9rem; }
  hr { border: 0; border-top: 1px solid var(--line); margin: 2.5rem 0 1rem; }
</style></head><body>
<p><span class="status">%s</span></p>
`, html.EscapeString(r.Project), colour, html.EscapeString(status))

	renderMarkdownish(w, md.String())
	fmt.Fprint(w, "\n</body></html>\n")
	return nil
}

// renderMarkdownish converts the subset of markdown this report emits.
//
// A full parser would be a dependency, and a report generator is not a place to acquire one:
// the output is written here, so the subset is known exactly.
func renderMarkdownish(w io.Writer, md string) {
	inTable := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		isRow := strings.HasPrefix(trimmed, "|")
		if inTable && !isRow {
			fmt.Fprint(w, "</table>\n")
			inTable = false
		}

		switch {
		case strings.HasPrefix(trimmed, "# "):
			fmt.Fprintf(w, "<h1>%s</h1>\n", inline(trimmed[2:]))
		case strings.HasPrefix(trimmed, "## "):
			fmt.Fprintf(w, "<h2>%s</h2>\n", inline(trimmed[3:]))
		case strings.HasPrefix(trimmed, "### "):
			fmt.Fprintf(w, "<h3>%s</h3>\n", inline(trimmed[4:]))
		case trimmed == "---":
			fmt.Fprint(w, "<hr>\n")
		case isRow:
			cells := splitRow(trimmed)
			if isSeparator(cells) {
				continue
			}
			if !inTable {
				fmt.Fprint(w, "<table>\n")
				inTable = true
				fmt.Fprint(w, "<tr>")
				for _, c := range cells {
					fmt.Fprintf(w, "<th>%s</th>", inline(c))
				}
				fmt.Fprint(w, "</tr>\n")
				continue
			}
			fmt.Fprint(w, "<tr>")
			for _, c := range cells {
				fmt.Fprintf(w, "<td>%s</td>", inline(c))
			}
			fmt.Fprint(w, "</tr>\n")
		case strings.HasPrefix(trimmed, "- "):
			fmt.Fprintf(w, "<p>&bull; %s</p>\n", inline(trimmed[2:]))
		case trimmed == "":
		default:
			fmt.Fprintf(w, "<p>%s</p>\n", inline(trimmed))
		}
	}
	if inTable {
		fmt.Fprint(w, "</table>\n")
	}
}

func splitRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return len(cells) > 0
}

// inline escapes first, then applies the two markers the report uses. Escaping afterwards would
// re-escape the tags this produces.
func inline(s string) string {
	out := html.EscapeString(s)
	out = replacePairs(out, "**", "<strong>", "</strong>")
	out = replacePairs(out, "`", "<code>", "</code>")
	return out
}

func replacePairs(s, marker, open, close string) string {
	var b strings.Builder
	opened := false
	for {
		i := strings.Index(s, marker)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		if opened {
			b.WriteString(close)
		} else {
			b.WriteString(open)
		}
		opened = !opened
		s = s[i+len(marker):]
	}
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

func orAll(s string) string {
	if s == "" {
		return "all agents"
	}
	return s
}
