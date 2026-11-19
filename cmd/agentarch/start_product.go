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
			fmt.Fprintln(os.Stderr, tf("product.needflag", flagName))
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
	if !need(&o.goal, t("product.ask.goal"), "goal") ||
		!need(&o.owner, t("product.ask.owner"), "owner") {
		return exitUsage
	}
	if o.profile == "" {
		if !o.interactive {
			fmt.Fprintf(os.Stderr, "%s\n%s", t("product.needprofile"), workflow.ProfileChoiceText())
			return exitUsage
		}
		chosen, ok := askProfile(o.root)
		if !ok {
			return exitUsage
		}
		o.profile = chosen
	}

	fmt.Printf("\n%s\n\n", strings.Repeat("─", 60))
	fmt.Printf("  %-15s %s\n", t("product.plan.name"), o.name)
	fmt.Printf("  %-15s %s\n", t("product.plan.goal"), o.goal)
	fmt.Printf("  %-15s %s\n", t("product.plan.owner"), o.owner)
	fmt.Printf("  %-15s %s\n", t("product.plan.profile"), o.profile)
	fmt.Printf("  %-15s %s\n", t("plan.installsinto"), displayRoot(o.root))
	fmt.Printf("  %-15s %s\n", t("product.plan.tech"), t("product.plan.tech.value"))
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
		_, err = s.Init(workflow.InitOptions{Name: o.name, Goal: o.goal, Profile: o.profile, Author: o.owner, Language: o.lang})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		if !isGitRepo(o.root) {
			fmt.Fprintln(os.Stderr, t("product.notgit"))
		}
		return exitUsage
	}

	fmt.Print(t("product.done"))
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
