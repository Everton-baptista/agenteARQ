// Command agentarch is the reference implementation of the agentarch standard.
//
// It is a reference implementation, not the standard itself. The normative contracts live in
// spec/, and spec/conformance/ exists so a second implementation can prove itself correct.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/i18n"
	"github.com/Everton-baptista/agenteARQ/internal/lockfile"
	"github.com/Everton-baptista/agenteARQ/internal/mcp"
	"github.com/Everton-baptista/agenteARQ/internal/render"
	"github.com/Everton-baptista/agenteARQ/internal/validate"
)

// Exit codes are normative — see spec/normative/06-exit-codes.md. They are distinct because CI
// must be able to tell "you forgot to run sync" apart from "your agent is unsafe"; collapsing
// them into a single failure teaches people to ignore both.
const (
	exitOK        = 0 // success
	exitUsage     = 1 // usage, internal, or version incompatibility
	exitStructure = 2 // structural validation failed
	exitDrift     = 3 // generated files are out of date
	exitGate      = 4 // a blocker-severity control failed
	exitWaiver    = 5 // a waiver is invalid or has expired
	exitRevalid   = 6 // a revalidation trigger fired without revalidation
)

// version is injected by the release build. A `go install` does not run those ldflags, so it
// falls back to the module version Go records in the binary — otherwise someone who installed
// v0.1.1 is told they are running a dev build, and has no way to know which.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		if v := info.Main.Version; v != "(devel)" {
			version = strings.TrimPrefix(v, "v")
		}
	}
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		// A bare `agentarch` in a project that has not installed the standard is almost always
		// somebody who just created the directory and wants to begin. Printing a wall of
		// subcommands answers a question they have not asked yet. Only on a terminal, and only
		// when there is nothing here to describe — a script that invokes us with no arguments
		// must still get usage and a non-zero exit rather than sitting on a prompt forever.
		if isTTY() {
			if s := detectState("."); !s.Installed {
				return cmdStart(nil)
			}
		}
		usage()
		return exitUsage
	}
	switch args[0] {
	case "start":
		return cmdStart(args[1:])
	case "init":
		return cmdInit(args[1:])
	case "sync":
		return cmdSync(args[1:])
	case "blueprint", "bp":
		return cmdBlueprint(args[1:])
	case "new":
		return cmdNew(args[1:])
	case "validate":
		return cmdValidate(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "explain":
		return cmdExplain(args[1:])
	case "waive":
		return cmdWaive(args[1:])
	case "mcp":
		return cmdMCP(args[1:])
	case "conformance":
		return cmdConformance(args[1:])
	case "score":
		return cmdScore(args[1:])
	case "pack":
		return cmdPack(args[1:])
	case "diff":
		return cmdDiff(args[1:])
	case "upgrade":
		return cmdUpgrade(args[1:])
	case "aibom":
		return cmdAIBOM(args[1:])
	case "report":
		return cmdReport(args[1:])
	case "version", "--version", "-v":
		specVer, _ := fs.ReadFile(agentarch.Spec, "spec/VERSION")
		fmt.Printf("agentarch %s\n%s\n", version, strings.TrimSpace(string(specVer)))
		return exitOK
	case "help", "--help", "-h":
		usage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		return exitUsage
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `agentarch — an open standard for building AI agents

  start       the one command to begin with: asks what you are building
              and does the rest. Everything below is the manual route.
  init        install the standard into this project
              --adopt   scan for agents that already exist and describe them
  blueprint   start from a complete, working project — run it with no
              arguments to choose from the catalogue
  new         scaffold an empty agent, tool or ADR from the templates
  sync        regenerate the assistant instruction files
              --check   report drift without writing (exit 3)
  validate    check artifacts for structure and consistency (exit 2)
              --strict-i18n  fail on a stale translation instead of warning
  check       the release gate: evaluate controls (exit 4 blocked, 5 waiver)
              --profile minimal|standard|regulated  --format text|json|sarif
              --explain-resolution  show which pack imposed each control
              --adopt-baseline      ratchet: block only on what is new or worse
              --update-baseline     drop entries you have since fixed
  explain     why a control exists and how to satisfy it
  waive       record a time-boxed, owned exception (max 90 days)
  mcp audit   check the MCP allowlist; --probe compares live tool descriptions
  conformance assess L1/L2/L3 and emit a badge that expires
  score       maturity by dimension, declared vs proven; never blocks
  pack        list|verify|add community packs (checksum verified, never executed)
  diff        which revalidation triggers fired since a git ref (--base)
  upgrade     replace agentarch/std, never touching agentarch/project
  aibom       what this agent is made of: models, prompts, corpora, tools, MCP
  report      everything the gate knows, as markdown and a self-contained page
  version     print the CLI and spec versions

Docs: https://github.com/Everton-baptista/agenteARQ
`)
}

// ---------------------------------------------------------------- init

func cmdInit(args []string) int {
	fs_ := flag.NewFlagSet("init", flag.ContinueOnError)
	profile := fs_.String("profile", "minimal", "policy profile: minimal, standard, regulated")
	lang := fs_.String("lang", "en", "language for the generated instruction files")
	root := fs_.String("root", ".", "project root")
	jurisdictions := fs_.String("jurisdictions", "",
		"comma-separated, e.g. EU,BR — selects which reg.* packs apply")
	adopt := fs_.Bool("adopt", false,
		"scan for agents that already exist and scaffold manifests describing them")
	adoptID := fs_.String("adopt-id", "", "id for the adopted agent (default: the directory name)")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	stdDir := filepath.Join(*root, "agentarch", "std")
	if err := copyEmbedded(agentarch.Content, "content", stdDir); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}
	if err := copyEmbedded(agentarch.Spec, "spec/schemas", filepath.Join(stdDir, "schemas")); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}
	// The normative contracts travel with the project too. An adopter who cannot read the
	// specification cannot check the tool against it, and "open standard" would mean "trust
	// the binary".
	if err := copyEmbedded(agentarch.Spec, "spec/normative", filepath.Join(stdDir, "spec")); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		return exitUsage
	}

	// Record what was installed. Without this, `upgrade` compares the installed tree against
	// the payload a newer binary carries, and every file the standard changed upstream looks
	// exactly like a file the project edited.
	if l, err := lockfile.Build(stdDir, version, time.Now().UTC().Format("2006-01-02")); err == nil {
		_ = lockfile.Write(stdDir, l)
	}

	for _, d := range []string{"agents", "tools", "mcp", "adr"} {
		if err := os.MkdirAll(filepath.Join(*root, "agentarch", "project", d), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "init:", err)
			return exitUsage
		}
	}

	cfg := filepath.Join(*root, "agentarch", "agentarch.yaml")
	if _, err := os.Stat(cfg); os.IsNotExist(err) {
		juris := "[]"
		if *jurisdictions != "" {
			var parts []string
			for _, j := range strings.Split(*jurisdictions, ",") {
				parts = append(parts, `"`+strings.ToUpper(strings.TrimSpace(j))+`"`)
			}
			juris = "[" + strings.Join(parts, ", ") + "]"
		}

		content := fmt.Sprintf(`schema_version: "1.0"
installed_version: "%s"
project:
  default_profile: %s
  lang: %s
  # Recorded once here; `+"`agentarch new agent`"+` copies it into each manifest, and the
  # reg.* packs resolve against it. An agent may declare more, never fewer.
  jurisdictions: %s

# Which assistant instruction files to generate. These are outputs: never edit them by
# hand — edit agentarch/std/core/ and run "agentarch sync".
sync:
  targets: [agents_md, claude, gemini]

# The release gate. Left empty until you have controls in place — a gate that blocks on
# day one gets switched off on day two.
gates:
  release:
    profile: %s
    fail_on: []
`, version, *profile, *lang, juris, *profile)
		if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "init:", err)
			return exitUsage
		}
	}

	// The content version, not the CLI's. They are separate version lines on purpose, and this
	// line used to print the binary's — so a local build announced "content 0.1.2+dirty" for a
	// content tree sitting at 1.0.0. The same confusion had already made `sync --check` fail
	// once, by stamping the CLI version into shim headers.
	fmt.Printf("installed agentarch/std (content %s, by agentarch %s) and agentarch/project\n",
		render.ContentVersion(os.DirFS(stdDir)), version)
	if code := cmdSync([]string{"--root", *root, "--lang", *lang}); code != exitOK {
		return code
	}
	if *adopt {
		return finishAdopt(*root, *adoptID)
	}
	// start composes init with the steps that follow it and prints one set of next steps at the
	// end. Two lists of next steps in one flow is how a guided setup stops feeling guided.
	if insideStart {
		return exitOK
	}

	fmt.Printf("\nNext:\n")
	fmt.Printf("  agentarch blueprint        start from a working project\n")
	fmt.Printf("  agentarch new agent <id>   or scaffold an empty one\n")
	fmt.Printf("  agentarch validate         structure and consistency\n")
	fmt.Printf("  agentarch check            the release gate\n\n")
	fmt.Printf("Commit the generated instruction files. They are outputs — edit\n")
	fmt.Printf("agentarch/std/core/ and re-run sync, and let CI run `sync --check`.\n")
	return exitOK
}

func copyEmbedded(src fs.FS, from, to string) error {
	return fs.WalkDir(src, from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}

// ---------------------------------------------------------------- sync

func cmdSync(args []string) int {
	fs_ := flag.NewFlagSet("sync", flag.ContinueOnError)
	check := fs_.Bool("check", false, "report drift without writing; exit 3 if any")
	root := fs_.String("root", ".", "project root")
	lang := fs_.String("lang", "en", "language of the generated files")
	targets := fs_.String("targets", "", "comma-separated subset of targets")
	if err := fs_.Parse(args); err != nil {
		return exitUsage
	}

	// Prefer the project's installed copy so that a project pinned to an older content
	// release keeps rendering that release. Falling back to the embedded payload is what
	// makes this repository able to host itself before anything is installed.
	//
	// Both are rooted at the content tree so BuildCore sees the same layout either way.
	source, err := fs.Sub(agentarch.Content, "content")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return exitUsage
	}
	if _, statErr := os.Stat(filepath.Join(*root, "agentarch", "std", "core")); statErr == nil {
		source = os.DirFS(filepath.Join(*root, "agentarch", "std"))
	}

	contentVersion := render.ContentVersion(source)

	core, err := render.BuildCore(source, *lang)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return exitUsage
	}
	if len(core.Text) > render.BudgetCore {
		fmt.Fprintf(os.Stderr,
			"AA-BUD-010  content/core/%s is %d bytes, over the %d byte budget by %d\n"+
				"    fix: remove an invariant or demote it to a standard. The budget is fixed on\n"+
				"    purpose — raising it is how the core grows until assistants stop reading it.\n",
			*lang, len(core.Text), render.BudgetCore, len(core.Text)-render.BudgetCore)
		return exitStructure
	}

	selected := render.Targets
	if *targets != "" {
		selected = nil
		for _, n := range strings.Split(*targets, ",") {
			t, ok := render.TargetByName(strings.TrimSpace(n))
			if !ok {
				fmt.Fprintf(os.Stderr, "sync: unknown target %q\n", n)
				return exitUsage
			}
			selected = append(selected, t)
		}
	} else if cfgTargets := readConfigTargets(*root); len(cfgTargets) > 0 {
		selected = cfgTargets
	}

	drift := 0

	// .mcp.json is derived from the reviewed allowlist rather than rendered from the core.
	// Keeping it generated is what makes the auditable document the source: two hand-kept
	// files agree right up until one of them is edited.
	// Respect the configured targets. The old condition also fired whenever the --targets flag
	// was absent, which meant a project that never asked for .mcp.json was told it had drifted.
	if wantsTarget(selected, "mcp_json") {
		n, code := syncMCPJSON(*root, *check)
		if code != exitOK {
			return code
		}
		drift += n
	}

	if wantsTarget(selected, "skills") {
		n, code := syncSkills(*root, *check)
		if code != exitOK {
			return code
		}
		drift += n
	}

	for _, t := range selected {
		if t.Name == "mcp_json" || t.Name == "skills" {
			continue
		}
		dst := filepath.Join(*root, t.Path)
		existing, _ := os.ReadFile(dst)

		out, err := render.Render(t, core, contentVersion, string(existing))
		if err != nil {
			fmt.Fprintf(os.Stderr, "AA-BUD-010  %s\n", err)
			return exitStructure
		}

		if string(existing) == out {
			continue
		}

		if *check {
			drift++
			reason := "content differs from the core"
			switch {
			case len(existing) == 0:
				reason = "file is missing"
			case render.CoreSHAOf(string(existing)) == "":
				reason = "file has no agentarch header — it was written by hand"
			case render.CoreSHAOf(string(existing)) != core.SHA256:
				reason = fmt.Sprintf("generated from core %s…, current core is %s…",
					core.SHA256[:8], render.CoreSHAOf(string(existing))[:8])
			}
			fmt.Fprintf(os.Stderr, "drift  %s\n    %s\n", t.Path, reason)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "sync:", err)
			return exitUsage
		}
		if err := os.WriteFile(dst, []byte(out), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "sync:", err)
			return exitUsage
		}
		fmt.Printf("wrote %s (%d/%d bytes)\n", t.Path, len(out), t.Budget)
	}

	if *check {
		if drift > 0 {
			fmt.Fprintf(os.Stderr,
				"\n%d generated file(s) out of date.\n"+
					"Run `agentarch sync`. Do not edit these files directly — put your own\n"+
					"additions between <!-- agentarch:custom:start --> and :end, which sync preserves.\n",
				drift)
			return exitDrift
		}
		fmt.Printf("all %d generated file(s) up to date (core %s…)\n", len(selected), core.SHA256[:8])
	}
	return exitOK
}

// readConfigTargets pulls sync.targets out of agentarch.yaml with a deliberately small parser:
// this runs before any schema is available, and a full YAML load here would make a
// misconfigured file fail in a way that is hard to explain.
func readConfigTargets(root string) []render.Target {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return nil
	}
	var out []render.Target
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "targets:") {
			continue
		}
		list := strings.Trim(strings.TrimPrefix(line, "targets:"), " []")
		for _, n := range strings.Split(list, ",") {
			if t, ok := render.TargetByName(strings.TrimSpace(n)); ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- validate

func cmdValidate(args []string) int {
	fs_ := flag.NewFlagSet("validate", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	strictI18n := fs_.Bool("strict-i18n", false, "treat a stale translation as an error, not a warning")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() > 0 {
		*root = fs_.Arg(0)
	}

	i18nProblems := checkTranslations(*root)
	contentFindings := lintContent(*root)

	v, err := validate.New(agentarch.Spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validate: cannot compile schemas:", err)
		return exitUsage
	}

	findings, err := v.Project(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "validate:", err)
		return exitUsage
	}
	for _, p := range i18nProblems {
		fmt.Fprintln(os.Stderr, p)
	}

	findings = append(findings, contentFindings...)

	if len(findings) == 0 {
		if len(i18nProblems) == 0 {
			fmt.Println("validate: no findings")
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "\n%d translation warning(s)\n", len(i18nProblems))
		if *strictI18n {
			return exitStructure
		}
		return exitOK
	}
	for _, f := range findings {
		fmt.Fprintln(os.Stderr, f)
	}
	fmt.Fprintf(os.Stderr, "\n%d finding(s)\n", len(findings))
	return exitStructure
}

// lintContent runs the checks that keep the standard itself honest: no framework named outside
// the adapters, and every adapter answering the same questions.
func lintContent(root string) []validate.Finding {
	src, err := contentFS(root)
	if err != nil {
		return nil
	}
	var out []validate.Finding
	if fs, err := validate.LintFrameworkNeutrality(src); err == nil {
		out = append(out, fs...)
	}
	if fs, err := validate.LintAdapterCoverage(src); err == nil {
		out = append(out, fs...)
	}
	if fs, err := validate.LintSkillChecklistParity(src); err == nil {
		out = append(out, fs...)
	}
	return out
}

// checkTranslations reports translations whose source has moved. Warnings by default: a project
// that installed the standard did not write the translations and cannot fix them, so failing its
// build over upstream drift would only teach it to stop running validate.
func checkTranslations(root string) []i18n.Status {
	src, err := contentFS(root)
	if err != nil {
		return nil
	}
	all, err := i18n.Check(src)
	if err != nil {
		return nil
	}
	return i18n.Problems(all)
}

// wantsTarget reports whether a target was selected.
func wantsTarget(sel []render.Target, name string) bool {
	for _, t := range sel {
		if t.Name == name {
			return true
		}
	}
	return false
}

// syncMCPJSON regenerates .mcp.json from the reviewed allowlist. Returns the number of files
// found to be out of date under --check.
func syncMCPJSON(root string, check bool) (int, int) {
	a, _, err := mcp.Load(root)
	if err != nil {
		return 0, exitOK // no allowlist, nothing to derive
	}
	want, err := mcp.RenderMCPJSON(a)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}
	want = append(want, '\n')

	dst := filepath.Join(root, ".mcp.json")
	existing, _ := os.ReadFile(dst)
	if string(existing) == string(want) {
		return 0, exitOK
	}
	if check {
		reason := "content differs from agentarch/project/mcp/allowlist.yaml"
		if len(existing) == 0 {
			reason = "file is missing"
		}
		fmt.Fprintf(os.Stderr, "drift  .mcp.json\n    %s\n", reason)
		return 1, exitOK
	}
	if err := os.WriteFile(dst, want, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}
	fmt.Printf("wrote .mcp.json (%d server(s) from the allowlist)\n", len(a.Servers))
	return 0, exitOK
}

// readEmbeddedContent reads one file from the payload this binary carries, using the same
// relative path an installed agentarch/std uses.
func readEmbeddedContent(rel string) ([]byte, error) {
	return fs.ReadFile(agentarch.Content, path.Join("content", filepath.ToSlash(rel)))
}

// syncSkills copies the skill directories into .claude/skills/.
//
// Copied rather than rendered: a skill is a document an assistant loads on demand, not part of
// the always-loaded core, so it has no budget and no core digest. The same workflows exist as
// checklists for assistants that do not load skills at all.
func syncSkills(root string, check bool) (int, int) {
	src, err := contentFS(root)
	if err != nil {
		return 0, exitOK
	}
	entries, err := fs.ReadDir(src, "skills")
	if err != nil {
		return 0, exitOK // no skills in this content tree
	}

	dest := filepath.Join(root, ".claude", "skills")
	drift := 0
	copied := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		err := fs.WalkDir(src, path.Join("skills", e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			body, rerr := fs.ReadFile(src, p)
			if rerr != nil {
				return rerr
			}
			rel := strings.TrimPrefix(p, "skills/")
			out := filepath.Join(dest, filepath.FromSlash(rel))

			existing, _ := os.ReadFile(out)
			if string(existing) == string(body) {
				return nil
			}
			if check {
				drift++
				reason := "content differs from agentarch/std/skills/"
				if len(existing) == 0 {
					reason = "file is missing"
				}
				fmt.Fprintf(os.Stderr, "drift  %s\n    %s\n", filepath.Join(".claude", "skills", rel), reason)
				return nil
			}
			if mkErr := os.MkdirAll(filepath.Dir(out), 0o755); mkErr != nil {
				return mkErr
			}
			mode := os.FileMode(0o644)
			if strings.HasSuffix(rel, ".py") || strings.HasSuffix(rel, ".sh") {
				mode = 0o755
			}
			copied++
			return os.WriteFile(out, body, mode)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "sync:", err)
			return drift, exitUsage
		}
	}
	if copied > 0 {
		fmt.Printf("wrote .claude/skills (%d file(s) from %d skill(s))\n", copied, len(entries))
	}
	return drift, exitOK
}

// finishAdopt scaffolds a manifest describing an agent that already exists.
//
// The adoption path matters more than it looks. A project with existing agents fails dozens of
// controls on day one; without a way in that costs nothing on the first day, the gate is
// switched off and nothing improves. That risk was named in the plan and this is the mitigation.
func finishAdopt(root, id string) int {
	if id == "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			id = "adopted-agent"
		} else {
			id = slugify(filepath.Base(abs))
		}
	}
	if !slugRe.MatchString(id) {
		id = "adopted-agent"
	}

	fmt.Printf("\nscanning for an agent that already exists…\n")
	scan := scanForAgents(root)

	if err := writeAdoptedManifest(root, id, scan); err != nil {
		fmt.Fprintln(os.Stderr, "adopt:", err)
		return exitUsage
	}

	rel := filepath.Join("agentarch", "project", "agents", id)
	fmt.Printf("\nwrote %s/agent.yaml\n\n", rel)

	switch {
	case len(scan.Providers) == 0 && len(scan.Models) == 0:
		fmt.Printf("Nothing recognisable was found, so almost everything is `unknown`.\n")
	default:
		if len(scan.Providers) > 0 {
			fmt.Printf("  providers   %s\n", strings.Join(scan.Providers, ", "))
		}
		if len(scan.Models) > 0 {
			shown := scan.Models
			if len(shown) > 5 {
				shown = shown[:5]
			}
			fmt.Printf("  models      %s\n", strings.Join(shown, ", "))
		}
		if len(scan.Frameworks) > 0 {
			fmt.Printf("  frameworks  %s\n", strings.Join(scan.Frameworks, ", "))
		}
		if len(scan.Prompts) > 0 {
			fmt.Printf("  prompts?    %s\n", strings.Join(scan.Prompts[:min(3, len(scan.Prompts))], ", "))
		}
	}

	fmt.Printf(`
Anything the scan could not determine is left as "unknown" on purpose. A
plausible-looking wrong value is worse than a blank one, because nobody
re-examines a field that is already filled in.

Next:

  1. Fill in the unknowns, starting with owner.accountable and out_of_scope.
     Describe the agent as it is TODAY, not as you would like it to be.
  2. agentarch check --adopt-baseline

The second one records today's failures as the starting point, so the gate
blocks only what is new or worse. Nothing is forgiven — the score still
counts it, and you close entries deliberately with --update-baseline.
`)
	return exitOK
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case !prevDash && b.Len() > 0:
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
