package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

type startProductOptions struct {
	root, lang, name, goal, owner, profile, juris string
	interactive, yes                              bool
}

// startProduct is the path for any software product: a website, an API, an app, a CLI, docs, a
// data pipeline, or an agent built from scratch. It installs the standards and opens the guided
// workflow. It chooses no technology: the brief records constraints and preferences, and the ADR
// compares options before the owner picks one.
func startProduct(o startProductOptions) int {
	need := func(value *string, question, flagName string) bool {
		if strings.TrimSpace(*value) != "" {
			return true
		}
		if !o.interactive {
			fmt.Fprintf(os.Stderr, "start --product: pass --%s when not on a terminal\n", flagName)
			return false
		}
		for attempt := 0; attempt < 3; attempt++ {
			if *value = strings.TrimSpace(ask(question)); *value != "" {
				return true
			}
		}
		fmt.Fprintln(os.Stderr, t("common.givingup"))
		return false
	}

	if o.name == "" {
		o.name = filepath.Base(mustAbs(o.root))
	}
	if !need(&o.goal, "\nWhat will people be able to do with it? One sentence, in your words:\n> ", "goal") ||
		!need(&o.owner, "\nWho is accountable for it? A person's name, not a team:\n> ", "owner") {
		return exitUsage
	}
	if o.profile == "" {
		if !o.interactive {
			fmt.Fprintf(os.Stderr, "start --product: pass --engineering-profile; none is chosen for you:\n%s", workflow.ProfileChoiceText())
			return exitUsage
		}
		chosen, ok := askProfile(o.root)
		if !ok {
			return exitUsage
		}
		o.profile = chosen
	}

	fmt.Printf("\n%s\n\n", strings.Repeat("─", 60))
	fmt.Printf("  %-15s %s\n", "product", o.name)
	fmt.Printf("  %-15s %s\n", "goal", o.goal)
	fmt.Printf("  %-15s %s\n", "accountable", o.owner)
	fmt.Printf("  %-15s %s\n", "verified as", o.profile)
	fmt.Printf("  %-15s %s\n", "installs into", displayRoot(o.root))
	fmt.Printf("  %-15s %s\n", "technology", "not chosen yet: options are compared with you in the ADR")
	if !o.yes {
		if !o.interactive {
			fmt.Fprintln(os.Stderr, "\nNothing written. Re-run with --yes.")
			return exitUsage
		}
		if !confirm(t("plan.confirm")) {
			fmt.Println(t("plan.nothing"))
			return exitOK
		}
	}

	insideStart = true
	defer func() { insideStart = false }()
	initArgs := []string{"--root", o.root, "--profile", "standard", "--lang", o.lang}
	if o.juris != "" {
		initArgs = append(initArgs, "--jurisdictions", o.juris)
	}
	if code := cmdInit(initArgs); code != exitOK {
		return code
	}
	s, err := workflow.New(o.root)
	if err == nil {
		_, err = s.Init(workflow.InitOptions{Name: o.name, Goal: o.goal, Profile: o.profile, Author: o.owner})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		if !isGitRepo(o.root) {
			fmt.Fprintln(os.Stderr, "The workflow tracks a Git repository: run `git init` here and try again.")
		}
		return exitUsage
	}

	fmt.Printf(`
Done. The standards are in agentarch/std and the product workflow is open.

What happens next, in order — each step leaves a file in agentarch/project/ and the
next one stays closed until it exists:

  1. brief.md     who it is for, the problem, success, constraints (including any
                  technology you require or rule out — or "no preference")
  2. spec.md      acceptance criteria AC-1, AC-2… observable from outside
  3. adr/0001     two or more technology options compared on benefits and costs;
                  you choose, and the ADR records why
  4. tasks.yaml   work mapped to acceptance criteria and checks
  5. verify       isolated checks plus the security floor, then independent review,
                  release and a production check

Do it with your assistant: agentarch plugin install <claude|codex|gemini> ...
then ask it to "start the product". It will propose options rather than pick
them. Or by hand: agentarch workflow status shows the next step at any time.
`)
	return exitOK
}

func isGitRepo(root string) bool {
	dir := mustAbs(root)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
