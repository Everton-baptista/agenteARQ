package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/emit"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
)

// cmdReport renders everything the gate knows into one document.
//
// It exists because the audience for this is not the person who ran the gate. `check` is for
// the engineer who can fix it; a report is for a reviewer, an auditor, or the same engineer six
// weeks later — none of whom will re-run the command to find out what it said.
func cmdReport(args []string) int {
	fs_ := flag.NewFlagSet("report", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	profile := fs_.String("profile", "", "policy profile")
	out := fs_.String("out", "", "directory to write into; omit for markdown on stdout")
	format := fs_.String("format", "both", "md, html or both")
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
		fmt.Fprintln(os.Stderr, "report:", err)
		return exitUsage
	}

	var all []policy.Result
	for _, e := range ev {
		all = append(all, e.Results...)
	}

	// A report describes the project as the gate sees it, so it applies the same baseline.
	// Reporting failures the gate is not blocking on, without saying they are baselined, would
	// make the two documents disagree about the same day.
	if b, err := policy.LoadBaseline(defaultBaselinePath(*root)); err == nil && b != nil {
		all = policy.ApplyBaseline(all, b)
		for i := range ev {
			ev[i].Results = policy.ApplyBaseline(ev[i].Results, b)
		}
	}

	waivers, _ := policy.LoadWaivers(waiversPath(*root))

	name := filepath.Base(mustAbs(*root))
	r := emit.Report{
		Project:     name,
		Profile:     *profile,
		GeneratedAt: now,
		Version:     version,
		Results:     all,
		Summary:     policy.Summarize(all),
		Conformance: policy.Assess(ev, gateRunsInCI(*root), syncIsClean(*root), now),
		Dimensions:  policy.Score(all),
		Waivers:     waivers,
		WaiverIssue: policy.CheckWaivers(waivers, now),
	}

	if *out == "" {
		return renderTo(os.Stdout, r, "md")
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		return exitUsage
	}

	written := 0
	for _, f := range []string{"md", "html"} {
		if *format != "both" && *format != f {
			continue
		}
		path := filepath.Join(*out, "agentarch-report."+f)
		file, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "report:", err)
			return exitUsage
		}
		code := renderTo(file, r, f)
		file.Close()
		if code != exitOK {
			return code
		}
		fmt.Printf("wrote %s\n", path)
		written++
	}
	if written == 0 {
		fmt.Fprintf(os.Stderr, "report: unknown format %q (md, html or both)\n", *format)
		return exitUsage
	}

	// A report never blocks. It describes; `check` decides.
	fmt.Printf("\nconformance %s · %d blocker(s) · %d waived · %d baselined\n",
		r.Conformance.Level, len(r.Summary.Blockers), r.Summary.Waived, r.Summary.Baselined)
	return exitOK
}

func renderTo(w *os.File, r emit.Report, format string) int {
	var err error
	if format == "html" {
		err = emit.HTML(w, r)
	} else {
		err = emit.Markdown(w, r)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		return exitUsage
	}
	return exitOK
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
