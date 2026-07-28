package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
	"gopkg.in/yaml.v3"
)

// cmdDiff compares manifests between two revisions and reports which revalidation triggers
// fired.
//
// This is the command that gives the standard its visible day-to-day value: a pull-request
// comment saying "this changed the system prompt of triage, so system_prompt_changed fired and
// the last validation predates it" is a thing a reviewer can act on, unlike a passing check.
func cmdDiff(args []string) int {
	fs_ := flag.NewFlagSet("diff", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	base := fs_.String("base", "", "git ref to compare against (required)")
	format := fs_.String("format", "text", "text or json")
	strict := fs_.Bool("strict", false, "exit 6 when a trigger fired without revalidation")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if *base == "" {
		fmt.Fprintln(os.Stderr, "usage: agentarch diff --base <git-ref>")
		return exitUsage
	}

	now := time.Now().UTC()
	agents, err := policy.LoadAgents(*root, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "diff:", err)
		return exitUsage
	}

	type report struct {
		Agent    string           `json:"agent"`
		Triggers []policy.Trigger `json:"triggers"`
		Due      bool             `json:"revalidation_due"`
		Reason   string           `json:"reason,omitempty"`
	}
	var reports []report
	overdue := 0

	for _, a := range agents {
		rel, err := filepath.Rel(*root, filepath.Join(a.Dir, "agent.yaml"))
		if err != nil {
			continue
		}
		prev, err := gitShow(*root, *base, rel)
		if err != nil {
			// A manifest that did not exist at base is a new agent. Everything about it
			// is new, but nothing was invalidated, so no trigger fires.
			continue
		}
		prevAgent, _ := prev["agent"].(map[string]any)
		headAgent, _ := a.Ctx["agent"].(map[string]any)

		triggers := policy.DetectTriggers(a.ID, prevAgent, headAgent)
		if len(triggers) == 0 {
			continue
		}
		due, reason := policy.RevalidationDue(headAgent, triggers, now)
		if due {
			overdue++
		}
		reports = append(reports, report{Agent: a.ID, Triggers: triggers, Due: due, Reason: reason})
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(reports)
	} else if len(reports) == 0 {
		fmt.Printf("diff: no revalidation triggers fired since %s\n", *base)
	} else {
		for _, r := range reports {
			fmt.Printf("\n%s — %d trigger(s) fired since %s\n\n", r.Agent, len(r.Triggers), *base)
			for _, t := range r.Triggers {
				fmt.Printf("  %s\n\n", t)
			}
			if r.Due {
				fmt.Printf("  REVALIDATION DUE: %s\n", r.Reason)
				fmt.Printf("  Re-run the evals, review the threat model, then update\n")
				fmt.Printf("  lifecycle.last_validated_at.\n\n")
			}
		}
	}

	if overdue > 0 && *strict {
		return exitRevalid
	}
	return exitOK
}

// gitShow reads one file as it stood at a revision.
//
// The path is prefixed with ./ so git resolves it relative to -C rather than to the repository
// root. Without that, running diff from anywhere but the top of the repo silently finds nothing
// and reports that no triggers fired — the worst possible failure for this command, because it
// looks exactly like success.
func gitShow(root, ref, path string) (map[string]any, error) {
	cmd := exec.Command("git", "-C", root, "show", ref+":./"+filepath.ToSlash(path))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var y any
	if err := yaml.Unmarshal(out, &y); err != nil {
		return nil, err
	}
	b, err := json.Marshal(y)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s at %s is not a mapping", path, ref)
	}
	return m, nil
}

// cmdUpgrade replaces the vendored standard while leaving the project's own artifacts alone.
func cmdUpgrade(args []string) int {
	fs_ := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	dryRun := fs_.Bool("dry-run", false, "report what would change without writing")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	stdDir := filepath.Join(*root, "agentarch", "std")
	if _, err := os.Stat(stdDir); err != nil {
		fmt.Fprintln(os.Stderr, "upgrade: no agentarch/std here — run `agentarch init` first")
		return exitUsage
	}

	// Local edits to std/ are lost on upgrade. Reporting them before overwriting is the
	// difference between an upgrade and a surprise: if several projects edit the same file,
	// the standard is wrong, not the projects.
	edited, err := locallyEdited(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "upgrade:", err)
		return exitUsage
	}
	if len(edited) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d vendored file(s) have been edited locally:\n\n", len(edited))
		for _, f := range edited {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
		fmt.Fprintf(os.Stderr, "\nThese changes will be lost. The supported places to customise are\n"+
			"agentarch/project/, waivers.yaml, your profile, and agentarch:custom regions in the\n"+
			"generated files. If several projects need the same edit here, the standard is wrong.\n\n")
		if !*dryRun {
			fmt.Fprintln(os.Stderr, "Re-run with --dry-run to review, or move the changes and try again.")
			return exitStructure
		}
	}

	if *dryRun {
		fmt.Printf("upgrade --dry-run: would replace %s with content %s\n", stdDir, version)
		fmt.Printf("agentarch/project/ and agentarch.yaml are never touched.\n")
		return exitOK
	}

	if err := os.RemoveAll(stdDir); err != nil {
		fmt.Fprintln(os.Stderr, "upgrade:", err)
		return exitUsage
	}
	if code := cmdInit([]string{"--root", *root}); code != exitOK {
		return code
	}
	fmt.Printf("upgraded agentarch/std to content %s\n", version)
	fmt.Printf("Run `agentarch check` — new controls may have entered warn mode.\n")
	return exitOK
}

// locallyEdited compares the vendored tree with the payload this binary carries.
func locallyEdited(root string) ([]string, error) {
	stdDir := filepath.Join(root, "agentarch", "std")
	var out []string

	err := filepath.WalkDir(stdDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(stdDir, p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "schemas"+string(os.PathSeparator)) {
			return nil // copied from spec/, compared separately
		}
		want, err := readEmbeddedContent(rel)
		if err != nil {
			return nil // a file the payload does not have is a local addition, not an edit
		}
		got, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if string(got) != string(want) {
			out = append(out, filepath.Join("agentarch", "std", rel))
		}
		return nil
	})
	return out, err
}
