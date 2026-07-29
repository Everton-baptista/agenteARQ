package validate

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// frameworkNames is the list AA-FWK-014 looks for outside content/adapters/.
//
// The claim that the core is framework-neutral is worth exactly as much as the check that
// enforces it. Without one, an example creeps into a standard, then a code sample, and two
// releases later the standard is coupled to a framework's release cycle and dies with it.
var frameworkNames = []string{
	"langchain", "langgraph", "llamaindex", "haystack", "crewai", "autogen",
	"semantic kernel", "semantic-kernel", "pydantic ai", "pydantic-ai",
	"agno", "dspy", "vercel ai sdk", "openai agents sdk", "agents sdk",
	"google adk", "agent development kit", "claude agent sdk", "langsmith",
	"langfuse", "phoenix", "openinference",
}

// nameRe matches a framework name as a whole word, case-insensitively.
var nameRe = func() *regexp.Regexp {
	quoted := make([]string, len(frameworkNames))
	for i, n := range frameworkNames {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\b`)
}()

// codeFenceRe strips fenced code, so a snippet inside an adapter-adjacent document does not
// trip the lint on an import line.
var codeFenceRe = regexp.MustCompile("(?s)```.*?```")

// LintFrameworkNeutrality reports framework names outside the places allowed to mention them.
//
// Allowed: content/adapters/ (that is what it is for), and content/references/ (external
// mappings, where naming the thing being mapped is the point).
func LintFrameworkNeutrality(fsys fs.FS) ([]Finding, error) {
	var out []Finding

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		if strings.HasPrefix(p, "adapters/") || strings.HasPrefix(p, "references/") {
			return nil
		}

		raw, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return rerr
		}
		body := codeFenceRe.ReplaceAllString(string(raw), "")

		seen := map[string]bool{}
		for _, m := range nameRe.FindAllString(body, -1) {
			lower := strings.ToLower(m)
			if seen[lower] {
				continue
			}
			seen[lower] = true
			out = append(out, Finding{
				ID:      "AA-FWK-014",
				File:    path.Join("content", p),
				Message: fmt.Sprintf("names the framework %q outside content/adapters/", m),
				Fix: "move the framework-specific guidance into content/adapters/<framework>.md. " +
					"A core that couples to a framework inherits its release cycle.",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Message < out[j].Message
	})
	return out, nil
}

// LintAdapterCoverage reports adapters that skip one of the five questions every adapter answers.
// An adapter missing the action guardrail section is the one that matters, and the one most
// likely to be left out.
func LintAdapterCoverage(fsys fs.FS) ([]Finding, error) {
	entries, err := fs.ReadDir(fsys, "adapters")
	if err != nil {
		return nil, nil // no adapters is not an error
	}

	required := []struct{ heading, why string }{
		{"system prompt", "where the versioned prompt lives"},
		{"permission", "where the tool permission check happens"},
		{"guardrail", "where the three guardrail points install"},
		{"Telemetry", "how a span is emitted"},
		{"Handoff", "where handoff and approval attach"},
	}

	var out []Finding
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
			continue
		}
		raw, rerr := fs.ReadFile(fsys, path.Join("adapters", e.Name()))
		if rerr != nil {
			return nil, rerr
		}
		body := strings.ToLower(string(raw))
		for _, r := range required {
			if !strings.Contains(body, strings.ToLower(r.heading)) {
				out = append(out, Finding{
					ID:      "AA-ADP-017",
					File:    path.Join("content", "adapters", e.Name()),
					Message: fmt.Sprintf("does not cover %s", r.why),
					Fix:     "every adapter answers the same five questions; see content/adapters/README.md",
				})
			}
		}
	}
	return out, nil
}

// LintSkillChecklistParity reports a workflow that exists for one kind of assistant and not the
// other.
//
// Skills are loaded automatically by assistants that support the format; checklists are the same
// workflow for everyone else. A skill with no checklist means the standard works better in one
// tool, which contradicts the promise it makes on its own front page.
func LintSkillChecklistParity(fsys fs.FS) ([]Finding, error) {
	skills, err := fs.ReadDir(fsys, "skills")
	if err != nil {
		return nil, nil
	}

	haveChecklist := map[string]bool{}
	if lists, err := fs.ReadDir(fsys, "checklists"); err == nil {
		for _, l := range lists {
			if strings.HasSuffix(l.Name(), ".md") && l.Name() != "README.md" {
				haveChecklist[strings.TrimSuffix(l.Name(), ".md")] = true
			}
		}
	}

	var out []Finding
	seen := map[string]bool{}
	for _, s := range skills {
		if !s.IsDir() {
			continue
		}
		// agentarch-new-agent ↔ new-agent.md
		short := strings.TrimPrefix(s.Name(), "agentarch-")
		seen[short] = true
		if !haveChecklist[short] {
			out = append(out, Finding{
				ID:      "AA-SKL-018",
				File:    path.Join("content", "skills", s.Name()),
				Message: "has no matching checklist",
				Fix: "add content/checklists/" + short + ".md with the same workflow. " +
					"Without it the standard works better in assistants that load skills.",
			})
		}
		if _, err := fs.Stat(fsys, path.Join("skills", s.Name(), "SKILL.md")); err != nil {
			out = append(out, Finding{
				ID:      "AA-SKL-018",
				File:    path.Join("content", "skills", s.Name()),
				Message: "has no SKILL.md",
			})
		}
	}
	for short := range haveChecklist {
		if !seen[short] {
			out = append(out, Finding{
				ID:      "AA-SKL-018",
				File:    path.Join("content", "checklists", short+".md"),
				Message: "has no matching skill",
				Fix:     "add content/skills/agentarch-" + short + "/SKILL.md, or remove the checklist",
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}
