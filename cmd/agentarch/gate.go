package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/emit"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
	"gopkg.in/yaml.v3"
)

// hoistFlags moves leading positional arguments after the flags.
//
// Go's flag package stops parsing at the first non-flag argument, so `waive <id> --owner x`
// silently drops every flag. Rejecting that ordering would be defensible; accepting it is
// kinder, because it is the ordering everyone types.
func hoistFlags(args []string) []string {
	var positional, flags []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i:]...)
			break
		}
		positional = append(positional, args[i])
	}
	return append(flags, positional...)
}

// contentFS returns the project's installed std when present, otherwise the embedded payload.
// A project pinned to an older content release must keep being judged by that release.
func contentFS(root string) (fs.FS, error) {
	if _, err := os.Stat(filepath.Join(root, "agentarch", "std", "packs")); err == nil {
		return os.DirFS(filepath.Join(root, "agentarch", "std")), nil
	}
	return fs.Sub(agentarch.Content, "content")
}

func waiversPath(root string) string {
	return filepath.Join(root, "agentarch", "project", "waivers.yaml")
}

// ---------------------------------------------------------------- check

func cmdCheck(args []string) int {
	fs_ := flag.NewFlagSet("check", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	profile := fs_.String("profile", "", "policy profile: minimal, standard, regulated")
	only := fs_.String("agent", "", "restrict to one agent id")
	format := fs_.String("format", "text", "text, json or sarif")
	sarifOut := fs_.String("sarif-output", "", "write SARIF to this file instead of stdout")
	explainRes := fs_.Bool("explain-resolution", false, "show which pack imposed each control")
	baselinePath := fs_.String("baseline", "", "ratchet mode: block only on what is new or worse")
	adoptBaseline := fs_.Bool("adopt-baseline", false, "record today's failures as the starting point")
	updateBaseline := fs_.Bool("update-baseline", false, "drop entries that are now fixed")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() > 0 {
		*root = fs_.Arg(0)
	}
	if *profile == "" {
		*profile = configProfile(*root)
	}
	if _, ok := policy.Profiles[*profile]; !ok {
		fmt.Fprintf(os.Stderr, "unknown profile %q (minimal, standard, regulated)\n", *profile)
		return exitUsage
	}

	now := time.Now().UTC()

	cfs, err := contentFS(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}
	cat, err := policy.LoadCatalog(cfs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check: cannot load the control catalogue:", err)
		return exitUsage
	}

	agents, err := policy.LoadAgents(*root, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	waivers, err := policy.LoadWaivers(waiversPath(*root))
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}
	waiverProblems := policy.CheckWaivers(waivers, now)

	var all []policy.Result
	fileOf := map[string]string{}

	for _, a := range agents {
		if *only != "" && a.ID != *only {
			continue
		}
		agentNode, _ := a.Ctx["agent"].(map[string]any)
		packIDs := policy.PacksFor(*profile, agentNode)

		res, missing, err := policy.ResolvePacks(cat, packIDs, agentNode)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			return exitUsage
		}
		for _, m := range missing {
			fmt.Fprintf(os.Stderr, "note: pack %q is referenced but not installed; its controls are not evaluated\n", m)
		}

		if *explainRes {
			fmt.Printf("\nresolution for %s (profile %s)\n", a.ID, *profile)
			for _, r := range res {
				line := fmt.Sprintf("  %-52s %-8s from %s@%s", r.ControlID, r.Severity, r.FromPack, r.Version)
				if len(r.Superseded) > 0 {
					line += "  (over " + strings.Join(r.Superseded, ", ") + ")"
				}
				fmt.Println(line)
			}
		}

		out := policy.Evaluate(cat, res, a.Ctx, waivers, now)
		rel, _ := filepath.Rel(*root, filepath.Join(a.Dir, "agent.yaml"))
		for _, r := range out {
			fileOf[r.AgentID+"\x00"+r.ControlID] = rel
		}
		all = append(all, out...)
	}

	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "check: no agents matched")
		return exitUsage
	}

	// Ratchet mode. A project with existing agents fails dozens of controls on day one; without
	// this the gate is switched off and nothing improves. With it, only what is new or worse
	// blocks, and the debt stays visible in `score` rather than being forgotten.
	if *baselinePath == "" {
		*baselinePath = defaultBaselinePath(*root)
	}
	baseline, err := policy.LoadBaseline(*baselinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		return exitUsage
	}

	if *adoptBaseline {
		b := policy.NewBaseline(all, *profile, now)
		if err := b.Save(*baselinePath); err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			return exitUsage
		}
		fmt.Printf("\nrecorded %d existing failure(s) as the baseline in %s\n\n",
			len(b.Accepted), *baselinePath)
		fmt.Printf("The gate now blocks only on what is new or worse than this.\n")
		fmt.Printf("Nothing here is forgiven — `agentarch score` still counts it, and\n")
		fmt.Printf("`agentarch check --update-baseline` removes each entry as you fix it.\n")
		return exitOK
	}

	if baseline != nil && *updateBaseline {
		next := baseline.Update(all, now)
		fixed := len(baseline.Accepted) - len(next.Accepted)
		if err := next.Save(*baselinePath); err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			return exitUsage
		}
		fmt.Printf("baseline updated: %d fixed, %d still open\n", fixed, len(next.Accepted))
		if fixed == 0 {
			fmt.Printf("Nothing closed since the last update.\n")
		}
		return exitOK
	}

	if baseline != nil {
		all = policy.ApplyBaseline(all, baseline)
	}

	sum := policy.Summarize(all)
	fileFor := func(r policy.Result) string {
		if f, ok := fileOf[r.AgentID+"\x00"+r.ControlID]; ok {
			return f
		}
		return "agentarch/project"
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"profile": *profile, "results": all,
			"summary": map[string]int{
				"total": sum.Total, "passed": sum.Passed, "waived": sum.Waived,
				"blockers": len(sum.Blockers), "majors": len(sum.Majors),
				"minors": len(sum.Minors), "warns": len(sum.Warns), "errors": len(sum.Errors),
			},
		})
	case "sarif":
		w := os.Stdout
		if *sarifOut != "" {
			f, err := os.Create(*sarifOut)
			if err != nil {
				fmt.Fprintln(os.Stderr, "check:", err)
				return exitUsage
			}
			defer f.Close()
			w = f
		}
		if err := emit.SARIF(w, all, version, fileFor); err != nil {
			fmt.Fprintln(os.Stderr, "check:", err)
			return exitUsage
		}
	default:
		printCheckText(all, sum, waiverProblems, *profile)
	}

	// Exit codes are distinct so CI can route them differently: a lapsed exception should
	// reach its owner, not alarm the whole team.
	if len(waiverProblems) > 0 {
		return exitWaiver
	}
	if len(sum.Blockers) > 0 || len(sum.Errors) > 0 {
		return exitGate
	}
	return exitOK
}

func printCheckText(all []policy.Result, sum policy.Summary, wp []policy.WaiverProblem, profile string) {
	byAgent := map[string][]policy.Result{}
	var order []string
	for _, r := range all {
		if _, seen := byAgent[r.AgentID]; !seen {
			order = append(order, r.AgentID)
		}
		byAgent[r.AgentID] = append(byAgent[r.AgentID], r)
	}
	sort.Strings(order)

	for _, id := range order {
		fmt.Printf("\n%s\n", id)
		rs := byAgent[id]
		sort.Slice(rs, func(i, j int) bool { return rs[i].ControlID < rs[j].ControlID })
		for _, r := range rs {
			switch {
			case r.Error != "":
				fmt.Printf("  ERROR   %s\n          %s\n", r.ControlID, r.Error)
			case r.Skipped:
				// Quiet by default: an inapplicable control is noise.
			case r.Passed:
			case r.Waived:
				fmt.Printf("  WAIVED  %s  until %s (%s)\n", r.ControlID, r.WaiverUntil, r.WaiverOwner)
			case r.Baselined:
				fmt.Printf("  DEBT    %s  baselined since %s\n", r.ControlID, r.BaselineSince)
			default:
				fmt.Printf("  %-7s %s\n          %s\n", strings.ToUpper(string(r.Severity)), r.ControlID, r.Message)
				if r.Remediation != "" {
					fmt.Printf("          fix: %s\n", r.Remediation)
				}
				fmt.Printf("          from %s@%s · %s\n", r.FromPack, r.PackVersion, r.StandardRef)
			}
		}
	}

	fmt.Printf("\nprofile %s · %d control(s) evaluated · %d passed · %d waived\n",
		profile, sum.Total, sum.Passed, sum.Waived)
	if sum.Baselined > 0 {
		fmt.Printf("%d baselined — not blocking, still counted against the score.\n"+
			"Run `agentarch check --update-baseline` as you close them.\n", sum.Baselined)
	}
	if n := len(sum.Blockers); n > 0 {
		fmt.Printf("%d blocker(s)\n", n)
	}
	if n := len(sum.Majors); n > 0 {
		fmt.Printf("%d major\n", n)
	}
	if n := len(sum.Warns); n > 0 {
		fmt.Printf("%d in grace period (not enforced yet)\n", n)
	}

	for _, p := range wp {
		fmt.Fprintf(os.Stderr, "\nWAIVER  %s (%s): %s\n", p.Waiver.Control, p.Waiver.Agent, p.Reason)
	}

	if len(sum.Blockers) > 0 {
		fmt.Fprintf(os.Stderr, "\nBlocked. Run `agentarch explain <control.id>` for the reasoning,\n"+
			"or `agentarch waive <control.id> --agent <id> --reason ... --owner ... --until YYYY-MM-DD`\n"+
			"to take the debt on deliberately.\n")
	}
}

// defaultBaselinePath is where a project's ratchet lives when one exists.
func defaultBaselinePath(root string) string {
	return filepath.Join(root, "agentarch", "project", "baseline.json")
}

func configProfile(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return "minimal"
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if v, found := strings.CutPrefix(line, "default_profile:"); found {
			return strings.TrimSpace(v)
		}
	}
	return "minimal"
}

// ---------------------------------------------------------------- explain

func cmdExplain(args []string) int {
	fs_ := flag.NewFlagSet("explain", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch explain <control.id>")
		return exitUsage
	}
	id := fs_.Arg(0)

	cfs, err := contentFS(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "explain:", err)
		return exitUsage
	}
	cat, err := policy.LoadCatalog(cfs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "explain:", err)
		return exitUsage
	}

	c, ok := cat.Controls[id]
	if !ok {
		fmt.Fprintf(os.Stderr, "no control %q. Close matches:\n", id)
		var names []string
		for k := range cat.Controls {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, n := range names {
			if strings.Contains(n, lastSegment(id)) {
				fmt.Fprintln(os.Stderr, "  "+n)
			}
		}
		return exitUsage
	}

	fmt.Printf("%s\n%s\n\n", c.ID, c.Title)
	fmt.Printf("Why it exists\n  %s\n\n", wrap(c.Intent, 76, "  "))
	fmt.Printf("How to fix it\n  %s\n\n", wrap(c.Remediation, 76, "  "))
	fmt.Printf("How it is checked\n  kind: %s\n", c.Check.Kind)
	if c.Check.Expr != "" {
		fmt.Printf("  expr: %s\n", c.Check.Expr)
	}
	fmt.Printf("\nEvidence         %s\n", strings.Join(c.Evidence, ", "))
	fmt.Printf("Status           %s\n", c.Status)
	fmt.Printf("Standard         %s\n", c.StandardRef)
	if len(c.References) > 0 {
		fmt.Printf("External         %s\n", strings.Join(c.References, ", "))
	}

	fmt.Printf("\nRequired by\n")
	found := false
	var packIDs []string
	for pid := range cat.Packs {
		packIDs = append(packIDs, pid)
	}
	sort.Strings(packIDs)
	for _, pid := range packIDs {
		p := cat.Packs[pid]
		for _, r := range p.Requires {
			if r.Control == id {
				found = true
				line := fmt.Sprintf("  %s@%s  %s", p.ID, p.Version, r.Severity)
				if r.EnforcedFrom != "" {
					line += fmt.Sprintf("  (enforced from %s)", r.EnforcedFrom)
				}
				fmt.Println(line)
				if r.Note != "" {
					fmt.Printf("      %s\n", wrap(r.Note, 72, "      "))
				}
			}
		}
	}
	if !found {
		fmt.Println("  no installed pack requires this control")
	}
	return exitOK
}

func lastSegment(s string) string {
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}

func wrap(s string, width int, indent string) string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for _, w := range words {
		if cur == "" {
			cur = w
		} else if len(cur)+1+len(w) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n"+indent)
}

// ---------------------------------------------------------------- waive

func cmdWaive(args []string) int {
	fs_ := flag.NewFlagSet("waive", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	agentID := fs_.String("agent", "", "agent id the waiver applies to")
	reason := fs_.String("reason", "", "why this is being accepted")
	owner := fs_.String("owner", "", "the person who will close it")
	until := fs_.String("until", "", "expiry, YYYY-MM-DD (required, max 90 days out)")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch waive <control.id> --agent <id> --reason ... --owner ... --until YYYY-MM-DD")
		return exitUsage
	}
	controlID := fs_.Arg(0)

	// Every field is mandatory. An exception without an owner is nobody's problem to close,
	// and one without an expiry is a quiet amendment to the standard.
	missing := []string{}
	if *reason == "" {
		missing = append(missing, "--reason")
	}
	if *owner == "" {
		missing = append(missing, "--owner")
	}
	if *until == "" {
		missing = append(missing, "--until")
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "waive requires %s\n", strings.Join(missing, ", "))
		return exitUsage
	}

	now := time.Now().UTC()
	d, err := time.Parse("2006-01-02", *until)
	if err != nil {
		fmt.Fprintln(os.Stderr, "waive: --until must be YYYY-MM-DD")
		return exitUsage
	}
	if !d.After(now) {
		fmt.Fprintln(os.Stderr, "waive: --until must be in the future")
		return exitUsage
	}
	if d.Sub(now) > policy.MaxWaiverDays*24*time.Hour {
		fmt.Fprintf(os.Stderr, "waive: --until is more than %d days away. "+
			"Pick a date you would actually defend at a review.\n", policy.MaxWaiverDays)
		return exitUsage
	}

	cfs, err := contentFS(*root)
	if err == nil {
		if cat, err := policy.LoadCatalog(cfs); err == nil {
			if _, ok := cat.Controls[controlID]; !ok {
				fmt.Fprintf(os.Stderr, "waive: no control %q\n", controlID)
				return exitUsage
			}
		}
	}

	path := waiversPath(*root)
	existing, _ := policy.LoadWaivers(path)
	for _, w := range existing {
		if w.Control == controlID && w.Agent == *agentID {
			fmt.Fprintf(os.Stderr, "waive: a waiver for %s on %s already exists (until %s)\n",
				controlID, *agentID, w.Until)
			return exitUsage
		}
	}
	existing = append(existing, policy.Waiver{
		Control: controlID, Agent: *agentID, Reason: *reason, Owner: *owner, Until: *until,
	})

	doc := map[string]any{"waivers": existing}
	buf, err := yaml.Marshal(doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "waive:", err)
		return exitUsage
	}
	header := "# Time-boxed exceptions. Every entry needs an owner and an expiry; `agentarch check`\n" +
		"# exits 5 once one lapses, so the debt surfaces rather than settling in.\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "waive:", err)
		return exitUsage
	}
	if err := os.WriteFile(path, append([]byte(header), buf...), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "waive:", err)
		return exitUsage
	}

	fmt.Printf("waived %s for %s until %s (owner %s)\n", controlID, *agentID, *until, *owner)
	fmt.Printf("recorded in %s\n", path)
	return exitOK
}
