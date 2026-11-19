package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Everton-baptista/agenteARQ/internal/application"
	"github.com/Everton-baptista/agenteARQ/internal/dashboard"
	"github.com/Everton-baptista/agenteARQ/internal/mcpserver"
	"github.com/Everton-baptista/agenteARQ/internal/plugin"
	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

func cmdWorkflow(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentarch workflow init|status|next|checkpoint|verify|review|record-pr|record-release|record-production-check [--root DIR] [--format json]")
		return exitUsage
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Println("usage: agentarch workflow init|status|next|checkpoint|verify|review|record-pr|record-release|record-production-check [--root DIR] [--format text|json]\nUse agentarch workflow <command> --help for its flags. Workflow does not publish PRs, merge, or deploy; the record-* commands record what a person or CI did.")
		return exitOK
	}
	command := args[0]
	f := flag.NewFlagSet("workflow "+command, flag.ContinueOnError)
	root := f.String("root", ".", "project Git worktree")
	format := f.String("format", "text", "text or json")
	name := f.String("name", "", "product name")
	profile := f.String("profile", "", "universal (default for a new product), typescript-web, python-api, go-service, static-site or docs")
	goal := f.String("goal", "", "observable product goal")
	author := f.String("author", "", "accountable implementer (never inferred)")
	ai := f.Bool("has-ai", false, "product contains AI components")
	executors := f.String("executors", "", "comma-separated development agent IDs")
	appAgents := f.String("application-agents", "", "comma-separated product agent IDs")
	phase := f.String("phase", "", "checkpoint phase")
	task := f.String("task", "", "completed task ID")
	note := f.String("note", "", "checkpoint note (no secrets)")
	rev := f.Uint64("expected-revision", 0, "expected current state revision; protects concurrent changes")
	image := f.String("image", "", "installed OCI verifier image pinned by @sha256 digest")
	diagnostics := f.String("diagnostics-reason", "", "explicit reason to capture bounded redacted test output; default retains only hashes")
	reviewer := f.String("reviewer", "", "independent reviewer identity")
	kind := f.String("kind", "human", "human or independent_agent")
	verdict := f.String("verdict", "", "approved or changes_requested")
	summary := f.String("summary", "", "review findings, or what the production check observed")
	prURL := f.String("url", "", "record-pr: the published pull request; record-release: the deployment run; record-production-check: the dashboard, log or report")
	head := f.String("head", "", "record-pr: exact commit of the pull request; record-release: the commit that was deployed")
	environment := f.String("environment", "", "record-release: target environment id, such as production")
	outcome := f.String("outcome", "", "record-production-check: passed or failed")
	attester := f.String("attester", "", "record-production-check: who checked production")
	if err := f.Parse(hoistFlags(args[1:])); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	if f.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional argument; use --root for the project directory")
		return exitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "format must be text or json")
		return exitUsage
	}
	diagnosticReason, diagnosticErr := diagnosticsFlagReason(f, *diagnostics)
	if diagnosticErr != nil || (diagnosticReason != "" && command != "verify") {
		if diagnosticErr == nil {
			diagnosticErr = errors.New("diagnostics-reason is only supported for workflow verify")
		}
		return printProduct(nil, diagnosticErr, *format)
	}
	if command == "status" {
		v, err := application.Status(*root)
		return printProduct(v, err, *format)
	}
	s, err := workflow.New(*root)
	if err != nil {
		return printProduct(nil, err, *format)
	}
	var expected *uint64
	f.Visit(func(v *flag.Flag) {
		if v.Name == "expected-revision" {
			expected = rev
		}
	})
	var result any
	switch command {
	case "init":
		result, err = s.Init(workflow.InitOptions{Name: *name, Profile: *profile, Goal: *goal, Author: *author, HasAI: *ai, ExecutorIDs: csv(*executors), ApplicationAgentIDs: csv(*appAgents)})
	case "next":
		result, err = s.Next()
	case "checkpoint":
		result, err = s.Checkpoint(workflow.CheckpointRequest{Phase: *phase, TaskID: *task, Note: *note, ExpectedRevision: expected})
	case "verify":
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		result, err = application.Verify(ctx, *root, workflow.VerifyOptions{Image: *image, ExpectedRevision: expected, DiagnosticsReason: diagnosticReason})
	case "review":
		result, err = s.Review(workflow.ReviewRequest{Reviewer: *reviewer, Kind: *kind, Verdict: *verdict, Summary: *summary, ExpectedRevision: expected})
	case "record-pr":
		result, err = s.RecordPR(workflow.PRRequest{URL: *prURL, Head: *head, ExpectedRevision: expected})
	case "record-release":
		result, err = s.RecordRelease(workflow.ReleaseRequest{URL: *prURL, Head: *head, Environment: *environment, ExpectedRevision: expected})
	case "record-production-check":
		result, err = s.RecordProductionCheck(workflow.ProductionCheckRequest{URL: *prURL, Outcome: *outcome, Attester: *attester, Summary: *summary, ExpectedRevision: expected})
	default:
		err = fmt.Errorf("unknown workflow command %q", command)
	}
	code := printProduct(result, err, *format)
	if code == 0 && command == "verify" {
		if snap, ok := result.(workflow.Snapshot); ok && (snap.State.Verification == nil || !snap.State.Verification.Passed) {
			return exitGate
		}
	}
	return code
}

func csv(s string) []string {
	out := []string{}
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func printProduct(v any, err error, format string) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentarch:", err)
		return exitUsage
	}
	if format == "json" {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		if e.Encode(v) != nil {
			return exitUsage
		}
		return 0
	}
	switch x := v.(type) {
	case workflow.Snapshot:
		fmt.Printf("\n%s · %s\nFase: %s · revisão %d\n\nPróximo passo: %s\n", x.Product.Name, x.Product.Profile, x.State.Phase, x.State.Revision, x.Next.Action)
		for _, m := range x.Next.Missing {
			fmt.Println("  •", m)
		}
		if x.State.Verification != nil {
			for _, c := range x.State.Verification.Checks {
				fmt.Printf("  %-13s %-30s %s\n", c.Status, c.ID, c.Message)
			}
		}
		if r := x.State.Release; r != nil {
			fmt.Printf("  release       %s at %s · %s\n", r.Environment, r.Head, r.URL)
		}
		if c := x.State.ProductionCheck; c != nil {
			fmt.Printf("  production    %s by %s (attested, not observed by agentarch) · %s\n", c.Outcome, c.Attester, c.URL)
		}
	case application.View:
		if x.Initialized && x.Snapshot != nil {
			return printProduct(*x.Snapshot, nil, format)
		}
		fmt.Println(x.Message)
	case workflow.NextAction:
		fmt.Println(x.Action)
		for _, m := range x.Missing {
			fmt.Println("  •", m)
		}
	case plugin.Report:
		fmt.Printf("Plugin %s · installed=%v · dry_run=%v\n", x.Host, x.Installed, x.DryRun)
		for _, c := range x.Changes {
			fmt.Printf("  %-12s %s\n", c.Action, c.Path)
		}
		for _, issue := range x.Issues {
			fmt.Println("  !", issue)
		}
	default:
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	}
	return 0
}

func cmdPlugin(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentarch plugin list|install|check|remove [--host codex|claude|gemini|external]")
		return exitUsage
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Println("usage: agentarch plugin list|install|check|remove [--host codex|claude|gemini|external] [--root DIR] [--format text|json]\nUse agentarch plugin <command> --help for its flags.")
		return exitOK
	}
	command := args[0]
	f := flag.NewFlagSet("plugin "+command, flag.ContinueOnError)
	root := f.String("root", ".", "project root")
	host := f.String("host", "", "coding assistant")
	format := f.String("format", "text", "text or json")
	dry := f.Bool("dry-run", false, "preview without writing")
	owner := f.String("owner", "", "named accountable person")
	contact := f.String("contact", "", "owner contact")
	provider := f.String("model-provider", "", "model provider explicitly selected by user")
	model := f.String("model", "", "exact pinned model identifier")
	binary := f.String("binary", "agentarch", "installed agentarch binary path")
	if err := f.Parse(hoistFlags(args[1:])); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "format must be text or json")
		return exitUsage
	}
	if f.NArg() > 1 || (f.NArg() > 0 && *host != "") {
		fmt.Fprintln(os.Stderr, "provide one host using --host or a single positional value")
		return exitUsage
	}
	if *host == "" && f.NArg() > 0 {
		*host = f.Arg(0)
	}
	if command == "list" {
		var all []plugin.Descriptor
		for _, h := range plugin.Hosts() {
			d, e := plugin.Describe(h)
			if e != nil {
				return printProduct(nil, e, *format)
			}
			all = append(all, d)
		}
		return printProduct(all, nil, *format)
	}
	opts := plugin.Options{DryRun: *dry, Owner: *owner, Contact: *contact, ModelProvider: *provider, ModelID: *model, Binary: *binary, ServerVersion: plugin.ServerVersion}
	var r plugin.Report
	var err error
	switch command {
	case "install":
		r, err = plugin.Install(*root, *host, opts)
	case "check":
		r, err = plugin.Check(*root, *host)
	case "remove":
		r, err = plugin.Remove(*root, *host, opts)
	default:
		err = fmt.Errorf("unknown plugin command %q", command)
	}
	code := printProduct(r, err, *format)
	if code == 0 && len(r.Issues) > 0 {
		return exitDrift
	}
	return code
}

func cmdMCPServe(args []string) int {
	f := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	root := f.String("root", ".", "fixed project root")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	resolved, err := mcpserver.ResolveRoot(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp:", err)
		return exitUsage
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := mcpserver.Serve(ctx, resolved, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcp:", err)
		return exitUsage
	}
	return 0
}

func cmdUI(args []string) int {
	f := flag.NewFlagSet("ui", flag.ContinueOnError)
	root := f.String("root", ".", "project root")
	port := f.Int("port", 0, "loopback port; 0 selects an available port")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return exitUsage
	}
	listener, server, url, err := dashboard.Listen(*root, *port)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	fmt.Printf("AgenteARQ · Painel local (somente leitura)\n%s\nCtrl+C para encerrar.\n", url)
	go func() {
		<-ctx.Done()
		closeCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = server.Shutdown(closeCtx)
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		return exitUsage
	}
	return 0
}
