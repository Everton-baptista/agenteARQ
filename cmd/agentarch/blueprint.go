package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/blueprint"
)

func cmdBlueprint(args []string) int {
	// No subcommand means "show me what there is and let me pick". Nobody arrives knowing
	// the id, and making them read a list elsewhere before they can act is friction with no
	// purpose.
	if len(args) == 0 {
		return cmdBlueprintAdd(nil)
	}
	switch args[0] {
	case "list":
		return cmdBlueprintList(args[1:])
	case "show":
		return cmdBlueprintShow(args[1:])
	case "add":
		return cmdBlueprintAdd(args[1:])
	default:
		// `agentarch blueprint rag-support` is what people type. Accept it.
		return cmdBlueprintAdd(append([]string{"--id"}, args...))
	}
}

func loadBlueprints(root string) ([]blueprint.Blueprint, error) {
	src, err := contentFS(root)
	if err != nil {
		return nil, err
	}
	return blueprint.Load(src)
}

func cmdBlueprintList(args []string) int {
	fs_ := flag.NewFlagSet("blueprint list", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	// Scripting against human-formatted output is a fragility that bites exactly once, in CI,
	// on a footer line nobody thought of.
	format := fs_.String("format", "text", "text, json, or ids")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	bps, err := loadBlueprints(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blueprint:", err)
		return exitUsage
	}
	if len(bps) == 0 {
		fmt.Println("no blueprints available")
		return exitOK
	}

	switch *format {
	case "ids":
		for _, b := range bps {
			fmt.Println(b.Meta.ID)
		}
		return exitOK
	case "json":
		metas := make([]blueprint.Meta, 0, len(bps))
		for _, b := range bps {
			metas = append(metas, b.Meta)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(metas)
		return exitOK
	}

	fmt.Printf("\nStarting points, by what you are trying to build\n\n")
	for _, b := range blueprint.ByNeed(bps) {
		fmt.Printf("  %-22s %s\n", b.Meta.ID, b.Meta.Need)
		fmt.Printf("  %-22s runs on: %s\n\n", "", strings.Join(b.Meta.Frameworks, ", "))
	}
	fmt.Printf("  agentarch blueprint            choose interactively\n")
	fmt.Printf("  agentarch blueprint show <id>  what it contains and why\n\n")
	return exitOK
}

func cmdBlueprintShow(args []string) int {
	fs_ := flag.NewFlagSet("blueprint show", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch blueprint show <id>")
		return exitUsage
	}
	bps, err := loadBlueprints(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blueprint:", err)
		return exitUsage
	}
	b, ok := blueprint.Find(bps, fs_.Arg(0))
	if !ok {
		fmt.Fprintf(os.Stderr, "no blueprint %q. Run `agentarch blueprint list`.\n", fs_.Arg(0))
		return exitUsage
	}

	fmt.Printf("\n%s — %s\n\n", b.Meta.ID, b.Meta.Title)
	fmt.Printf("%s\n\n", wrap(b.Meta.Description, 76, ""))
	fmt.Printf("  need          %s\n", b.Meta.Need)
	fmt.Printf("  system type   %s\n", b.Meta.SystemType)
	fmt.Printf("  runs on       %s\n", strings.Join(b.Meta.Frameworks, ", "))
	if b.Meta.Conformance != "" {
		fmt.Printf("  conformance   %s out of the box\n", b.Meta.Conformance)
	}
	if len(b.Meta.Demonstrates) > 0 {
		fmt.Printf("\n  What it shows you how to do:\n")
		for _, d := range b.Meta.Demonstrates {
			fmt.Printf("    · %s\n", d)
		}
	}
	fmt.Println()
	return exitOK
}

func cmdBlueprintAdd(args []string) int {
	fs_ := flag.NewFlagSet("blueprint add", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	id := fs_.String("id", "", "blueprint id; omit to choose interactively")
	framework := fs_.String("framework", "", "which runnable code to write")
	yes := fs_.Bool("yes", false, "skip the confirmation")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if *id == "" && fs_.NArg() == 1 {
		*id = fs_.Arg(0)
	}

	bps, err := loadBlueprints(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blueprint:", err)
		return exitUsage
	}
	if len(bps) == 0 {
		fmt.Fprintln(os.Stderr, "no blueprints available")
		return exitUsage
	}

	interactive := isTTY()

	// Choose the blueprint.
	var b blueprint.Blueprint
	switch {
	case *id != "":
		var ok bool
		if b, ok = blueprint.Find(bps, *id); !ok {
			fmt.Fprintf(os.Stderr, "no blueprint %q. Available:\n", *id)
			for _, x := range bps {
				fmt.Fprintf(os.Stderr, "  %-22s %s\n", x.Meta.ID, x.Meta.Need)
			}
			return exitUsage
		}
	case !interactive:
		fmt.Fprintln(os.Stderr,
			"not a terminal, so there is nobody to ask. Pass --id <blueprint>.\n"+
				"Run `agentarch blueprint list` to see the ids.")
		return exitUsage
	default:
		picked, code := chooseBlueprint(bps)
		if code != exitOK {
			return code
		}
		b = picked
	}

	// Choose the framework, but only ask when there is a real choice to make.
	fw := *framework
	switch {
	case fw != "" && !b.HasFramework(fw):
		fmt.Fprintf(os.Stderr,
			"blueprint %s does not ship code for %q.\nIt ships: %s\n"+
				"Porting it is documented in agentarch/std/adapters/%s.md\n",
			b.Meta.ID, fw, strings.Join(b.Meta.Frameworks, ", "), fw)
		return exitUsage
	case fw == "" && len(b.Meta.Frameworks) == 1:
		fw = b.Meta.Frameworks[0]
	case fw == "" && interactive:
		var code int
		if fw, code = chooseFramework(b); code != exitOK {
			return code
		}
	case fw == "":
		fw = b.Meta.Frameworks[0]
	}

	plan, err := blueprint.Prepare(mustContentFS(*root), b, *root, fw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blueprint:", err)
		return exitUsage
	}

	fmt.Printf("\n%s — %s\n", b.Meta.ID, b.Meta.Title)
	fmt.Printf("running on %s · %d file(s)\n\n", plan.Framework, len(plan.Files))
	for _, f := range plan.Files {
		fmt.Printf("  %s\n", f)
	}

	if len(plan.Conflicts) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d file(s) already exist and would be overwritten:\n\n", len(plan.Conflicts))
		for _, c := range plan.Conflicts {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Path)
		}
		fmt.Fprintf(os.Stderr, "\nNothing was written. Move them, or install into an empty directory\n"+
			"with --root, and try again.\n")
		return exitStructure
	}

	if !*yes {
		if !interactive {
			fmt.Fprintln(os.Stderr, "\nNothing written. Re-run with --yes to install.")
			return exitUsage
		}
		if !confirm("\nWrite these files?") {
			fmt.Println("nothing written")
			return exitOK
		}
	}

	if err := blueprint.Apply(mustContentFS(*root), b, *root, plan); err != nil {
		fmt.Fprintln(os.Stderr, "blueprint:", err)
		return exitUsage
	}

	// A blueprint can bring artifacts that generated files derive from — an MCP allowlist, for
	// one. Leaving the project one command short of being in sync means the first thing it does
	// is fail its own CI.
	if code := cmdSync([]string{"--root", *root}); code != exitOK {
		fmt.Fprintln(os.Stderr, "blueprint: installed, but sync failed — run `agentarch sync`")
	}

	fmt.Printf("\ninstalled %s\n\n", b.Meta.ID)
	fmt.Printf("It already passes the gate. Verify, then start editing:\n\n")
	fmt.Printf("  agentarch validate\n")
	fmt.Printf("  agentarch check --profile standard\n")
	fmt.Printf("  cat app/README.md      how to run it\n\n")
	fmt.Printf("The manifest is the contract. Change what the agent does there first,\n")
	fmt.Printf("then make the code match — `agentarch check` will tell you when they disagree.\n")
	return exitOK
}

// ---------------------------------------------------------------- prompting

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	// /dev/null is a character device too, and it is exactly what a CI runner hands a step for
	// stdin. Counting it as a terminal means the interview asks its questions into a void, reads
	// EOF, and takes the default answer to every one of them — so a run that should have stopped
	// and said "there is nobody to ask" installs something nobody chose, and reports success.
	if nul, nerr := os.Stat(os.DevNull); nerr == nil && os.SameFile(fi, nul) {
		return false
	}
	return true
}

var stdin = bufio.NewReader(os.Stdin)

func ask(prompt string) string {
	fmt.Print(prompt)
	line, err := stdin.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func confirm(prompt string) bool {
	answer := strings.ToLower(ask(prompt + " [y/N] "))
	return answer == "y" || answer == "yes"
}

func chooseBlueprint(bps []blueprint.Blueprint) (blueprint.Blueprint, int) {
	ordered := blueprint.ByNeed(bps)

	fmt.Printf("\nWhat are you building?\n\n")
	for i, b := range ordered {
		fmt.Printf("  %d. %s\n", i+1, b.Meta.Need)
		fmt.Printf("     %s — runs on %s\n\n", b.Meta.ID, strings.Join(b.Meta.Frameworks, ", "))
	}

	for attempt := 0; attempt < 3; attempt++ {
		in := ask(fmt.Sprintf("Choose 1–%d (or q to quit): ", len(ordered)))
		if in == "q" || in == "" {
			fmt.Println("nothing written")
			return blueprint.Blueprint{}, exitOK
		}
		n, err := strconv.Atoi(in)
		if err == nil && n >= 1 && n <= len(ordered) {
			return ordered[n-1], exitOK
		}
		// Accept the id too — someone who already knows it should not have to count.
		if b, ok := blueprint.Find(bps, in); ok {
			return b, exitOK
		}
		fmt.Fprintf(os.Stderr, "  not a choice on the list\n")
	}
	fmt.Fprintln(os.Stderr, "giving up after three tries; nothing written")
	return blueprint.Blueprint{}, exitUsage
}

func chooseFramework(b blueprint.Blueprint) (string, int) {
	fmt.Printf("\nThis blueprint ships runnable code for:\n\n")
	for i, f := range b.Meta.Frameworks {
		note := ""
		if f == "none" {
			note = "  (the provider SDK directly, no framework)"
		}
		fmt.Printf("  %d. %s%s\n", i+1, f, note)
	}
	fmt.Println()

	for attempt := 0; attempt < 3; attempt++ {
		in := ask(fmt.Sprintf("Choose 1–%d [1]: ", len(b.Meta.Frameworks)))
		if in == "" {
			return b.Meta.Frameworks[0], exitOK
		}
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(b.Meta.Frameworks) {
			return b.Meta.Frameworks[n-1], exitOK
		}
		if b.HasFramework(in) {
			return in, exitOK
		}
		fmt.Fprintln(os.Stderr, "  not one of the options")
	}
	return "", exitUsage
}

func mustContentFS(root string) fs.FS {
	src, err := contentFS(root)
	if err != nil {
		return nil
	}
	return src
}
