package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

// Effect: write, limited to disposable OCI snapshots. No workflow, plugin, approval,
// repository artifact or model session is created by this measurement command.
func cmdVerifyProfile(args []string) int {
	f := flag.NewFlagSet("verify-profile", flag.ContinueOnError)
	root := f.String("root", ".", "Git worktree to measure without initializing a workflow")
	profile := f.String("profile", "", "typescript-web or python-api")
	image := f.String("image", "", "prepared OCI image pinned by repository digest")
	diagnostics := f.String("diagnostics-reason", "", "explicit reason to capture bounded redacted test output; default retains only hashes")
	format := f.String("format", "text", "text or json")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if f.NArg() != 0 || (*format != "json" && *format != "text") {
		fmt.Fprintln(os.Stderr, "use --root, --profile, --image and --format text|json")
		return exitUsage
	}
	reason, err := diagnosticsFlagReason(f, *diagnostics)
	if err != nil {
		return printProduct(nil, err, *format)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	v, err := workflow.VerifyProfileWithDiagnostics(ctx, *root, *profile, *image, reason)
	if err != nil {
		return printProduct(nil, err, *format)
	}
	if *format == "json" {
		if code := printProduct(v, nil, "json"); code != 0 {
			return code
		}
	} else {
		for _, c := range v.Checks {
			fmt.Printf("%-13s %-20s %s\n", c.Status, c.ID, c.Message)
		}
	}
	if !v.Passed {
		return exitGate
	}
	return exitOK
}

func diagnosticsFlagReason(flags *flag.FlagSet, value string) (string, error) {
	provided := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "diagnostics-reason" {
			provided = true
		}
	})
	value = strings.TrimSpace(value)
	if provided && value == "" {
		return "", errors.New("diagnostics-reason requires an explicit nonempty reason")
	}
	return value, workflow.ValidateDiagnosticsReason(value)
}
