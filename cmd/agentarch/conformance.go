package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

// gatherEvidence assembles what the conformance assessment needs from a project.
func gatherEvidence(root, profile string, now time.Time) ([]policy.AgentEvidence, error) {
	cfs, err := contentFS(root)
	if err != nil {
		return nil, err
	}
	cat, err := policy.LoadCatalog(cfs)
	if err != nil {
		return nil, err
	}
	agents, err := policy.LoadAgents(root, now)
	if err != nil {
		return nil, err
	}
	waivers, _ := policy.LoadWaivers(waiversPath(root))

	var out []policy.AgentEvidence
	for _, a := range agents {
		ag, _ := a.Ctx["agent"].(map[string]any)
		res, _, err := policy.ResolvePacks(cat, policy.PacksFor(profile, ag), ag)
		if err != nil {
			return nil, err
		}

		ev := policy.AgentEvidence{
			ID:       a.ID,
			Manifest: a.Manifest,
			Results:  policy.Evaluate(cat, res, a.Ctx, waivers, now),
		}

		if evalNode, ok := a.Ctx["evals"].(map[string]any); ok {
			ev.EvalCompletedAt, _ = evalNode["completed_at"].(string)
			if rt, ok := evalNode["redteam"].(map[string]any); ok {
				ev.RedTeamExecuted, _ = rt["executed"].(bool)
			}
			if ms, ok := evalNode["metrics"].([]any); ok {
				for _, m := range ms {
					if mm, ok := m.(map[string]any); ok {
						ev.EvalMetrics = append(ev.EvalMetrics, mm)
					}
				}
			}
		}
		if e, ok := ag["evaluation"].(map[string]any); ok {
			if v, ok := e["max_result_age_days"].(float64); ok {
				ev.EvalMaxAgeDays = int(v)
			}
		}
		if obs, ok := ag["observability"].(map[string]any); ok {
			if otel, ok := obs["otel"].(map[string]any); ok {
				ev.OTelEnabled, _ = otel["enabled"].(bool)
				sv, _ := otel["semconv_version"].(string)
				ev.SemconvPinned = sv != ""
			}
		}
		if links, ok := ag["links"].(map[string]any); ok {
			if tm, _ := links["threat_model"].(string); tm != "" {
				_, statErr := os.Stat(filepath.Join(a.Dir, tm))
				ev.HasThreatModel = statErr == nil
			}
		}

		out = append(out, ev)
	}
	return out, nil
}

// gateRunsInCI looks for the gate in a committed workflow. A gate that exists only on a laptop
// is a linter, not a gate — the whole claim of L2 is that the rules block a merge.
func gateRunsInCI(root string) bool {
	for _, p := range []string{
		".github/workflows", ".gitlab-ci.yml", ".circleci/config.yml", "azure-pipelines.yml",
	} {
		full := filepath.Join(root, p)
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if b, err := os.ReadFile(full); err == nil && strings.Contains(string(b), "agentarch check") {
				return true
			}
			continue
		}
		entries, _ := os.ReadDir(full)
		for _, e := range entries {
			if b, err := os.ReadFile(filepath.Join(full, e.Name())); err == nil &&
				strings.Contains(string(b), "agentarch check") {
				return true
			}
		}
	}
	return false
}

func cmdConformance(args []string) int {
	fs_ := flag.NewFlagSet("conformance", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	profile := fs_.String("profile", "", "policy profile")
	badge := fs_.Bool("badge", false, "emit a shields.io endpoint document")
	badgeOut := fs_.String("badge-output", "", "write the badge JSON to this file")
	format := fs_.String("format", "text", "text or json")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() > 0 {
		*root = fs_.Arg(0)
	}
	if *profile == "" {
		*profile = configProfile(*root)
	}
	now := time.Now().UTC()

	ev, err := gatherEvidence(*root, *profile, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "conformance:", err)
		return exitUsage
	}

	inSync := syncIsClean(*root)
	c := policy.Assess(ev, gateRunsInCI(*root), inSync, now)

	if *badge || *badgeOut != "" {
		b, _ := json.MarshalIndent(c.Badge(), "", "  ")
		if *badgeOut != "" {
			if err := os.WriteFile(*badgeOut, append(b, '\n'), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "conformance:", err)
				return exitUsage
			}
			fmt.Printf("wrote %s\n", *badgeOut)
		} else {
			fmt.Println(string(b))
		}
		if !*badge {
			return exitOK
		}
	}

	if *format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(c)
		return exitOK
	}

	fmt.Printf("\nagentarch conformance: %s   (%d agent(s), profile %s)\n", c.Level, c.Agents, *profile)
	if c.ExpiresAt != "" {
		fmt.Printf("valid until %s — the earliest date a piece of evidence goes stale.\n", c.ExpiresAt)
		fmt.Printf("After that this drops to L2 on its own; conformance that never decays is advertising.\n")
	}

	current := policy.Level("")
	for _, r := range c.Requirements {
		if r.Level != current {
			current = r.Level
			fmt.Printf("\n%s\n", r.Level)
		}
		mark := "ok  "
		if !r.Met {
			mark = "MISS"
		}
		line := fmt.Sprintf("  %s  %s", mark, r.Text)
		if !r.Met && r.Details != "" {
			line += "\n        " + r.Details
		}
		fmt.Println(line)
	}

	fmt.Println()
	return exitOK
}

// syncIsClean reports whether the generated instruction files match the core.
func syncIsClean(root string) bool {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer devnull.Close()

	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devnull, devnull
	code := cmdSync([]string{"--check", "--root", root})
	os.Stdout, os.Stderr = stdout, stderr
	return code == exitOK
}

func cmdScore(args []string) int {
	fs_ := flag.NewFlagSet("score", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	profile := fs_.String("profile", "", "policy profile")
	format := fs_.String("format", "text", "text, md or json")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() > 0 {
		*root = fs_.Arg(0)
	}
	if *profile == "" {
		*profile = configProfile(*root)
	}
	now := time.Now().UTC()

	ev, err := gatherEvidence(*root, *profile, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "score:", err)
		return exitUsage
	}
	var all []policy.Result
	for _, e := range ev {
		all = append(all, e.Results...)
	}
	dims := policy.Score(all)

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(dims)
	case "md":
		fmt.Println("| Dimension | Declared | Proven | Controls |")
		fmt.Println("|---|---:|---:|---:|")
		for _, d := range dims {
			fmt.Printf("| %s | %.0f%% | %.0f%% | %d |\n", d.Name, d.Declared, d.Proven, d.Total)
		}
	default:
		fmt.Printf("\nmaturity by control type (profile %s, %d agent(s))\n\n", *profile, len(ev))
		fmt.Printf("  %-12s %10s %10s %8s\n", "", "declared", "proven", "controls")
		for _, d := range dims {
			proven := fmt.Sprintf("%.0f%%", d.Proven)
			if d.Proven == 0 && d.Declared > 0 {
				proven = "—"
			}
			fmt.Printf("  %-12s %9.0f%% %10s %8d\n", d.Name, d.Declared, proven, d.Total)
		}
		fmt.Printf("\n  declared: satisfied by a field someone wrote down.\n")
		fmt.Printf("  proven:   satisfied by an artifact that had to be produced.\n")
		fmt.Printf("  Kept apart on purpose — collapsing them is how \"we are compliant\" comes\n")
		fmt.Printf("  to mean nothing. This never blocks a release.\n\n")
	}
	return exitOK
}
