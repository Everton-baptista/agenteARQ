package main

// `agentarch start` — one guided entry point.
//
// Everything start does could already be done with init, blueprint and check. It exists because
// the first command a person runs must not require the vocabulary the standard is about to teach
// them. Before this, the first thing anyone typed was:
//
//     agentarch init --profile standard --jurisdictions BR
//
// which asks a newcomer to decide what a profile is and why a jurisdiction matters before
// anything at all has happened. Three concepts, none of them the reason they came. So start asks
// in ordinary language — is the code already written, what are you building, where are your users
// — and derives the flags from the answers.
//
// At most three questions, and it asks nothing it can find out for itself: whether the directory
// already has code, whether the blueprint ships more than one framework. A question with an
// obvious answer trains people to stop reading the questions.
//
// It asks nothing about the person running it, and reads nothing about them. Every question is
// about the software being built.

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/blueprint"
	"github.com/Everton-baptista/agenteARQ/internal/render"
	"gopkg.in/yaml.v3"
)

// insideStart suppresses the "Next:" footers that init and the adopt path print on their own.
// start composes those steps into one flow and prints a single set of next steps at the end;
// three competing lists of next steps is how a guided flow stops feeling guided.
var insideStart bool

// projectState is what the directory tells us before we ask anybody anything.
type projectState struct {
	Installed bool // agentarch/agentarch.yaml exists
	Agents    int  // manifests under agentarch/project/agents/
	HasCode   bool // source files that are not ours
}

func detectState(root string) projectState {
	var s projectState
	if _, err := os.Stat(filepath.Join(root, "agentarch", "agentarch.yaml")); err == nil {
		s.Installed = true
	}
	if entries, err := os.ReadDir(filepath.Join(root, "agentarch", "project", "agents")); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, "agentarch", "project", "agents", e.Name(), "agent.yaml")); err == nil {
				s.Agents++
			}
		}
	}
	s.HasCode = hasSourceFiles(root)
	return s
}

// hasSourceFiles reports whether there is code here that we did not write. It decides only which
// answer is offered as the default — a wrong guess costs one keystroke, so it stays shallow and
// cheap rather than trying to be clever.
func hasSourceFiles(root string) bool {
	found := false
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	var dirs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if skipDirs[name] || strings.HasPrefix(name, ".") {
				continue
			}
			dirs = append(dirs, filepath.Join(root, name))
			continue
		}
		if isSourceName(name) {
			found = true
		}
	}
	if found {
		return true
	}
	// One level down, because src/ or app/ is where the code usually is.
	for _, d := range dirs {
		sub, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range sub {
			if !e.IsDir() && isSourceName(e.Name()) {
				return true
			}
		}
	}
	return false
}

func isSourceName(name string) bool {
	switch filepath.Ext(name) {
	case ".py", ".ts", ".tsx", ".js", ".go", ".rb", ".java", ".cs", ".rs", ".kt":
		return true
	}
	return false
}

func cmdStart(args []string) int {
	fs_ := flag.NewFlagSet("start", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	// Every answer has a flag, so that what the interview does by hand can also be done by a
	// script — a setup path that only exists interactively cannot be tested or automated.
	productFlag := fs_.Bool("product", false, "skip the first question: a software product of any kind, built through the guided workflow")
	productName := fs_.String("name", "", "with --product: the product name (default: the directory name)")
	productGoal := fs_.String("goal", "", "with --product: what users will be able to do")
	engProfile := fs_.String("engineering-profile", "", "with --product: how it is verified (omit on a terminal to compare the options)")
	newProject := fs_.Bool("new", false, "skip the first question: a new AI agent from a blueprint")
	adopt := fs_.Bool("adopt", false, "skip the first question: the code already exists")
	refactorFlag := fs_.Bool("refactor", false, "adopt an existing AI agent, and install the refactoring workflow")
	forceAgent := fs_.Bool("agent", false, "with --adopt: describe an agent even when no model SDK or framework is found")
	bpID := fs_.String("blueprint", "", "which starting point to install")
	framework := fs_.String("framework", "", "which runnable code to write")
	providerFlag := fs_.String("provider", "", "model provider: "+strings.Join(providerIDs(), ", "))
	jurisdictions := fs_.String("jurisdictions", "", "comma-separated, e.g. EU,BR")
	owner := fs_.String("owner", "", "the person accountable for the product or agent")
	contact := fs_.String("contact", "", "how to reach them")
	profile := fs_.String("profile", "standard", "minimal, standard or regulated")
	// Named langFlag, not lang: `lang` is the package variable the catalogue reads, and a local
	// shadowing it would leave the interview speaking English while the files came out
	// translated — the two would silently disagree about what was chosen.
	langFlag := fs_.String("lang", SourceLang, "language of the interview and the generated files")
	yes := fs_.Bool("yes", false, "do not ask before writing")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}

	// Check the flag before anything is written. Passing --jurisdictions brasil used to reach the
	// manifest untouched, fail `validate` on the schema pattern, and be reported as a bug in the
	// blueprint — an error arriving detached from the answer that caused it, blaming the wrong
	// party. Reject it here, where the fix is obvious.
	if *jurisdictions != "" {
		codes, ok := parseJurisdictions(*jurisdictions)
		if !ok {
			fmt.Fprintf(os.Stderr,
				"--jurisdictions %q is not a list of country codes.\n"+
					"Use ISO 3166-1 alpha-2, or EU: --jurisdictions BR   --jurisdictions EU,BR\n",
				*jurisdictions)
			return exitUsage
		}
		*jurisdictions = codes
	}

	interactive := isTTY()
	state := detectState(*root)

	// Already installed and already describing something. Re-running init here would be a no-op
	// that looks like progress, so say what is here and name the command that actually helps.
	if state.Installed && state.Agents > 0 {
		printNextSteps(state)
		return exitOK
	}

	// ---- Question 0: which language.
	//
	// It comes before the greeting, because a greeting has to be written in some language and
	// picking one before asking is how a tool tells somebody it was not built for them. The
	// answer is the same value --lang takes, so it reaches the generated instruction files —
	// every one of them, for every assistant, not just the one that happens to be running.
	switch {
	case !render.ValidLanguage(*langFlag):
		fmt.Fprintf(os.Stderr, "--lang %q is not a language tag; any language works (for example %s, es, fr, ja)\n",
			*langFlag, strings.Join(languageCodes(), ", "))
		return exitUsage
	case langSet(fs_):
		// Passed explicitly: honour it and do not ask. `pt` gets the Portuguese interview.
		*langFlag = resolveLanguage(*langFlag)
		lang = *langFlag
	case interactive:
		chosen, quit := askLanguage()
		if quit {
			return exitOK
		}
		lang = chosen
		*langFlag = chosen
	}

	// Only greet somebody who is there. A script driving this by flags gets the plan and the
	// result; an invitation to press Enter is noise in a CI log.
	if interactive {
		fmt.Printf("\n%s\n\n%s\n", formatBanner(), bold(t("banner.body")))
	}

	// ---- Question 1: what are you doing?
	adoptPath := *adopt || *refactorFlag
	refactorPath := *refactorFlag
	productPath := *productFlag
	if !adoptPath && !*newProject && !productPath {
		if !interactive {
			fmt.Fprintln(os.Stderr,
				"not a terminal, so there is nobody to ask. Pass one of:\n"+
					"  --product --goal \"...\" --owner \"...\" --engineering-profile <id>   any software product\n"+
					"  --new --blueprint <id> --provider <id>                          a new AI agent\n"+
					"  --adopt                                                         code that already exists\n"+
					"  --refactor                                                      an existing AI agent, to the standard\n"+
					"Run `agentarch blueprint list` for blueprint ids.")
			return exitUsage
		}
		mode, quit, code := askStartMode(state)
		if quit || code != exitOK {
			return code
		}
		productPath = mode == modeProduct
		adoptPath = mode == modeAdopt || mode == modeRefactor
		refactorPath = mode == modeRefactor
	}
	if productPath {
		return startProduct(startProductOptions{
			root: *root, lang: *langFlag, name: *productName, goal: *productGoal, owner: *owner,
			profile: *engProfile, juris: *jurisdictions, interactive: interactive, yes: *yes,
		})
	}

	// ---- Question 2 and 3: what are you building, and on what. Only for a new project: an
	// existing one already answered both, in code.
	var bp blueprint.Blueprint
	var prov providerChoice
	fw := *framework
	if !adoptPath {
		bps, err := loadBlueprints(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "start:", err)
			return exitUsage
		}
		if len(bps) == 0 {
			fmt.Fprintln(os.Stderr, "start: no blueprints available in this content tree")
			return exitUsage
		}
		switch {
		case *bpID != "":
			var ok bool
			if bp, ok = blueprint.Find(bps, *bpID); !ok {
				fmt.Fprintf(os.Stderr, "no blueprint %q. Available:\n", *bpID)
				for _, x := range bps {
					fmt.Fprintf(os.Stderr, "  %-22s %s\n", x.Meta.ID, x.Meta.Need)
				}
				return exitUsage
			}
		case !interactive:
			fmt.Fprintln(os.Stderr, "start: pass --blueprint <id> when not on a terminal.")
			return exitUsage
		default:
			var code int
			if bp, code = chooseBlueprint(bps); code != exitOK {
				return code
			}
			if bp.Meta.ID == "" {
				return exitOK // chose to quit
			}
		}

		switch {
		case fw != "" && !bp.HasFramework(fw):
			fmt.Fprintf(os.Stderr,
				"blueprint %s does not ship code for %q.\nIt ships: %s\n",
				bp.Meta.ID, fw, frameworkValues(bp.Meta.Frameworks))
			return exitUsage
		case fw == "" && len(bp.Meta.Frameworks) == 1:
			fw = bp.Meta.Frameworks[0]
		case fw == "" && interactive:
			var code int
			if fw, code = chooseFramework(bp); code != exitOK {
				return code
			}
		case fw == "":
			fmt.Fprintf(os.Stderr, "start: blueprint %s ships more than one framework; pass --framework (%s).\n",
				bp.Meta.ID, frameworkValues(bp.Meta.Frameworks))
			return exitUsage
		}

		// ---- Question 3b: which provider. Only for a new project, and only after the blueprint
		// is known — an adopted project already answered this in its own code, and re-pointing it
		// at a different provider is a refactor, not a setup step.
		switch {
		case *providerFlag != "":
			var ok bool
			if prov, ok = findProvider(*providerFlag); !ok {
				fmt.Fprintf(os.Stderr, "--provider %q is not one this project implements (%s)\n",
					*providerFlag, strings.Join(providerIDs(), ", "))
				return exitUsage
			}
		case interactive:
			var quit bool
			if prov, quit = askProvider(); quit {
				return exitOK
			}
		default:
			fmt.Fprintf(os.Stderr, "start: pass --provider when not on a terminal; none is chosen for you (%s).\n",
				strings.Join(providerIDs(), ", "))
			return exitUsage
		}
	}

	// ---- Question 4: where are the users? This is the jurisdiction question, asked without the
	// word — what a person knows is where their users are, not which regulatory packs that
	// resolves to.
	juris := *jurisdictions
	if juris == "" && interactive {
		var quit bool
		if juris, quit = askJurisdictions(); quit {
			return exitOK
		}
	}

	// The interview ends here. It asks nothing about the person running it — not the accountable
	// owner, not an address, and it does not read `git config` to guess either.
	//
	// The owner used to be question 5, on the reasoning that `owner.accountable` is the field the
	// gate leans on hardest. That was governance paperwork wedged into a getting-started flow, and
	// it did not even achieve the thing it was there for: a name typed in three seconds to get
	// past a prompt is exactly as unexamined as the placeholder it replaced. It belongs with
	// `purpose` and `out_of_scope` — the edits the closing text names, made while looking at the
	// manifest, by someone who has decided.
	//
	// --owner and --contact still work when passed, for scripts that do know.

	// ---- What will happen. Nothing has touched disk yet.
	fmt.Printf("\n%s\n\n", strings.Repeat("─", 60))
	switch {
	case refactorPath:
		fmt.Printf("%s\n\n", t("plan.refactoring"))
	case adoptPath:
		fmt.Printf("%s\n\n", t("plan.adopting"))
	default:
		fmt.Printf("%s — %s\n", bp.Meta.ID, blueprintTitle(bp))
		fmt.Printf("%s\n\n", tf("plan.runningon", frameworkLabel(fw)))
	}
	if prov.ID != "" {
		// The model id is shown, not just the provider name. It is what invariant 7 is about, it
		// is what the bill is denominated in, and a value that lands in a manifest without being
		// displayed is one nobody re-examines.
		fmt.Printf("  %-15s %s   %s\n", t("plan.provider"), prov.Label, prov.ModelID)
	}
	fmt.Printf("  %-15s %s\n", t("plan.installsinto"), displayRoot(*root))
	fmt.Printf("  %-15s %s   %s\n", t("plan.strictness"), *profile, t("plan.strictness.note"))
	// Shown only when passed, because only then is it written. Anything that lands in a manifest
	// without being displayed is a value nobody re-examines.
	if *owner != "" {
		fmt.Printf("  %-15s %s\n", t("plan.accountable"), *owner)
	}
	if *contact != "" {
		fmt.Printf("  %-15s %s\n", t("plan.contact"), *contact)
	}
	fmt.Printf("  %-15s %s\n", t("plan.regulation"), describeJurisdictions(juris))
	if state.Installed {
		// init never overwrites an existing agentarch.yaml, which is the right behaviour and the
		// wrong surprise: the answers still reach the manifests, where the reg.* packs resolve
		// from, but the file-level settings stay as they were. Say so rather than let it be found.
		fmt.Printf("\n  agentarch/agentarch.yaml is already here and is kept as it is —\n")
		fmt.Printf("  the answers above go into the manifest.\n")
	}

	var plan *blueprint.Plan
	if !adoptPath {
		var err error
		plan, err = blueprint.Prepare(mustContentFS(*root), bp, *root, fw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "start:", err)
			return exitUsage
		}
		fmt.Printf("  %-15s %s\n", t("plan.writes"), tf("plan.writes.files", len(plan.Files)))

		if len(plan.Conflicts) > 0 {
			fmt.Fprintf(os.Stderr, "\n%d file(s) already exist here and would be overwritten:\n\n", len(plan.Conflicts))
			for _, c := range plan.Conflicts {
				fmt.Fprintf(os.Stderr, "  %s\n", c.Path)
			}
			fmt.Fprintf(os.Stderr, "\nNothing was written. Move them aside, or run this in an empty\n"+
				"directory with --root, and try again.\n")
			return exitStructure
		}
	}

	if !*yes {
		if !interactive {
			fmt.Fprintln(os.Stderr, "\n"+t("start.nothing_written"))
			return exitUsage
		}
		if !confirm(t("plan.confirm")) {
			fmt.Println(t("plan.nothing"))
			return exitOK
		}
	}

	// ---- Do it.
	insideStart = true
	defer func() { insideStart = false }()

	fmt.Println()
	initArgs := []string{"--root", *root, "--profile", *profile, "--lang", *langFlag}
	if juris != "" {
		initArgs = append(initArgs, "--jurisdictions", juris)
	}
	if code := cmdInit(initArgs); code != exitOK {
		return code
	}

	agentIDs := []string{}
	if adoptPath {
		id := slugify(filepath.Base(mustAbs(*root)))
		if !slugRe.MatchString(id) {
			id = "adopted-agent"
		}
		scan := scanForAgents(*root)
		if !scan.IsAgent() && !*forceAgent {
			reportNotAnAgent(*root)
			return exitOK
		}
		if err := writeAdoptedManifest(*root, id, scan); err != nil {
			fmt.Fprintln(os.Stderr, "start:", err)
			return exitUsage
		}
		agentIDs = append(agentIDs, id)
		fmt.Printf("wrote agentarch/project/agents/%s/agent.yaml\n", id)
		reportScan(scan)
	} else {
		if err := blueprint.Apply(mustContentFS(*root), bp, *root, plan); err != nil {
			fmt.Fprintln(os.Stderr, "start:", err)
			return exitUsage
		}
		fmt.Printf("wrote %d file(s) from %s\n", len(plan.Files), bp.Meta.ID)
		if code := cmdSync([]string{"--root", *root, "--lang", *langFlag}); code != exitOK {
			fmt.Fprintln(os.Stderr, "start: installed, but sync failed — run `agentarch sync`")
		}
	}

	// The provider reaches three places, and all three have to agree: model.provider and model.id
	// in every manifest, and the pinned SDK in app/requirements.txt. A manifest naming one provider
	// beside a requirements file pinning another's SDK fails at the first model call, in a
	// traceback that blames the import rather than the setup.
	if prov.ID != "" {
		touched, err := applyProvider(*root, prov)
		if err != nil {
			fmt.Fprintln(os.Stderr, "start:", err)
		} else if touched > 0 {
			fmt.Printf("set %s / %s in %d file(s)\n", prov.ID, prov.ModelID, touched)
		}
	}

	// The answers only matter if they reach the manifests. A blueprint ships an example owner and
	// an example jurisdiction list, and an adopted manifest ships `unknown`; leaving either in
	// place would mean the interview was theatre.
	changed, err := personalizeManifests(*root, *owner, *contact, juris)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
	} else if changed > 0 {
		fmt.Printf("set the accountable person and jurisdictions in %d manifest(s)\n", changed)
	}

	// ---- Show where it stands, from the tool itself rather than from a promise in this output.
	fmt.Printf("\n%s\n", strings.Repeat("─", 60))
	if adoptPath {
		return finishStartAdopt(*root, agentIDs, refactorPath)
	}
	return finishStartNew(*root, bp, *profile, *owner != "", prov.Key)
}

// printNextSteps says where this directory stands and names at most four commands that move it
// forward from here.
//
// It exists because the alternative was `usage()`: twenty commands and an exit 1, printed to
// somebody who had just run `init` and had not yet made an agent. Being midway through a setup is
// not an error, and a list of everything the tool can do is not an answer to "what now" — it is
// the question restated in more detail.
func printNextSteps(state projectState) {
	switch {
	case state.Agents > 0:
		fmt.Printf("\n%s %s\n\n", iconCheck(), bold(green(fmt.Sprintf("agentarch is installed here, describing %d agent(s).", state.Agents))))
		fmt.Printf("  %-25s %s\n", formatCmd("agentarch check"), dim("the release gate"))
		fmt.Printf("  %-25s %s\n", formatCmd("agentarch conformance"), dim("L1 / L2 / L3, with an expiry"))
		fmt.Printf("  %-25s %s\n", formatCmd("agentarch blueprint"), dim("add another starting point"))
		fmt.Printf("  %-25s %s\n\n", formatCmd("agentarch new agent"), dim("scaffold an empty one"))

	default:
		fmt.Printf("\n%s %s\n\n", iconWarn(), bold(yellow("agentarch is installed here; no product workflow or agent is described yet.")))
		fmt.Printf("  %-25s %s\n", formatCmd("agentarch start --product"), dim("any software product, step by step"))
		fmt.Printf("  %-25s %s\n", formatCmd("agentarch workflow status"), dim("where a product already stands"))
		fmt.Printf("  %-25s %s\n\n", formatCmd("agentarch blueprint"), dim("an AI agent from a complete, working project"))
		fmt.Printf("Full command list: %s\n\n", cyan("agentarch --help --all"))
	}
}

// startBanner is kept only for the tests that assert the old wording; the interview reads the
// catalogue now.
const startBanner = `
agentarch — let's get you set up.

A few questions in plain language; nothing is written until you say so.
Press Enter to take the default, or q to quit.
`

// startMode is the answer to the first question.
type startMode int

const (
	modeProduct startMode = iota + 1
	modeAgent
	modeAdopt
	modeRefactor
)

// askStartMode asks what the person wants to do. It has no default: the four answers lead to
// different work, and pressing Enter is not a decision. The last value is an exit code; quitting
// returns quit=true so the interview stops instead of falling through.
func askStartMode(state projectState) (mode startMode, quit bool, code int) {
	hint := t("existing.hint.empty")
	if state.HasCode {
		hint = t("existing.hint.hascode")
	}
	fmt.Printf("\n%s\n\n", bold(t("existing.question")))
	fmt.Printf("%s\n", formatStep(1, t("existing.product")))
	fmt.Printf("%s\n", formatStep(2, t("existing.new")))
	fmt.Printf("%s\n", formatStep(3, t("existing.adopt")))
	fmt.Printf("%s\n\n", formatStep(4, t("existing.refactor")))
	fmt.Printf("%s\n", dim("("+hint+")"))

	for attempt := 0; attempt < 3; attempt++ {
		switch strings.ToLower(ask(t("existing.prompt"))) {
		case "1", "product", "produto":
			return modeProduct, false, exitOK
		case "2", "agent", "agente", "new", "novo":
			return modeAgent, false, exitOK
		case "3", "existing", "adopt", "existe", "continuar":
			return modeAdopt, false, exitOK
		case "4", "refactor", "refatorar":
			return modeRefactor, false, exitOK
		case "q", "quit", "sair":
			fmt.Println(t("plan.nothing"))
			return 0, true, exitOK
		}
		fmt.Fprintln(os.Stderr, t("common.notanoption"))
	}
	fmt.Fprintln(os.Stderr, t("common.givingup"))
	return 0, true, exitUsage
}

// askExisting is the adoption view of askStartMode, kept for callers that only distinguish
// adopting from refactoring.
func askExisting(state projectState) (adopt, refactor, quit bool, code int) {
	mode, quit, code := askStartMode(state)
	return mode == modeAdopt || mode == modeRefactor, mode == modeRefactor, quit, code
}

// askJurisdictions asks where the users are and returns what --jurisdictions wants.
//
// The question is deliberately about people and not about law. Someone building a support agent
// knows where their customers are; whether that means LGPD or GDPR is the standard's job to
// work out, and stating the consequence next to each option is how the concept gets taught
// without being a prerequisite.
func askJurisdictions() (juris string, quit bool) {
	// Brazil and Europe are named first because they are the two with regulatory packs written,
	// and saying so is honest. They are not the only answers: anyone can type their own country
	// code, and the standards apply everywhere regardless — only the reg.* packs are regional.
	// A menu that offers a person in Austin, Lagos or Tokyo nothing but "somewhere else" tells
	// them the tool is not for them, which is the regional-bias failure the design set out to
	// avoid.
	fmt.Printf("\n%s\n\n", bold(t("juris.question")))
	fmt.Printf("  1. %-26s %s\n", bold(t("juris.brazil")), dim(t("juris.brazil.note")))
	fmt.Printf("  2. %-26s %s\n", bold(t("juris.europe")), dim(t("juris.europe.note")))
	fmt.Printf("  3. %s\n", bold(t("juris.both")))
	fmt.Printf("  4. %-26s %s\n", bold(t("juris.other")), dim(t("juris.other.note")))
	fmt.Printf("  5. %s\n\n", bold(t("juris.undecided")))
	fmt.Printf("%s\n\n", dim(t("juris.explain")))

	for attempt := 0; attempt < 3; attempt++ {
		in := ask(t("juris.prompt"))
		switch strings.ToLower(in) {
		case "1", "brazil", "brasil":
			return "BR", false
		case "2", "europe", "europa":
			return "EU", false
		case "3", "both", "ambos":
			return "EU,BR", false
		case "", "5":
			return "", false
		case "q", "quit", "sair":
			fmt.Println(t("plan.nothing"))
			return "", true
		case "4":
			fmt.Fprintln(os.Stderr, t("juris.typecode"))
			continue
		}
		if codes, ok := parseJurisdictions(in); ok {
			return codes, false
		}
		fmt.Fprintln(os.Stderr, t("juris.notacode"))
	}
	return "", false
}

// parseJurisdictions accepts what the manifest schema accepts: ISO 3166-1 alpha-2 codes, or EU.
// Anything else is rejected here rather than written out and failing `validate` later, where the
// error would arrive detached from the answer that caused it.
func parseJurisdictions(s string) (string, bool) {
	var out []string
	for _, part := range strings.Split(s, ",") {
		code := strings.ToUpper(strings.TrimSpace(part))
		if len(code) != 2 || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
			return "", false
		}
		out = append(out, code)
	}
	if len(out) == 0 {
		return "", false
	}
	return strings.Join(out, ","), true
}

// describeJurisdictions names the packs a set of codes resolves to, so the consequence of the
// answer is visible before anything is written rather than discovered later in a gate failure.
func describeJurisdictions(j string) string {
	codes := strings.Split(strings.ToUpper(strings.ReplaceAll(j, " ", "")), ",")
	var packs, unmapped []string
	for _, c := range codes {
		switch c {
		case "":
		case "BR":
			packs = append(packs, "reg.br-lgpd")
		case "EU":
			packs = append(packs, "reg.gdpr", "reg.eu-ai-act")
		default:
			unmapped = append(unmapped, c)
		}
	}
	switch {
	case len(packs) == 0 && len(unmapped) == 0:
		return "none declared — add them to the manifest whenever you know"
	case len(packs) == 0:
		// Declared and honest: no pack exists for it yet, and the standards apply anyway. Saying
		// "none" here would read as "the rules do not apply to you", which is the opposite.
		return strings.Join(unmapped, ", ") + " declared — no legal pack ships for it yet"
	case len(unmapped) == 0:
		return strings.Join(packs, ", ")
	}
	return strings.Join(packs, ", ") + " · " + strings.Join(unmapped, ", ") + " has no pack yet"
}

func displayRoot(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(abs, home) {
		return "~" + strings.TrimPrefix(abs, home)
	}
	return abs
}

// ------------------------------------------------------- writing the answers in

// Provider adaptation still uses scalar line matching; interview answers use a YAML tree.
var fieldRe = regexp.MustCompile(`^(\s*)([a-z_]+):[ \t]*([^#\n]*)(#.*)?$`)

// manifestNode follows a mapping path without confusing equally named nested fields.
// A yaml.Node round-trip preserves the manifest's comments and scalar values, including hashes.
func manifestNode(node *yaml.Node, path ...string) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	for _, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	return node
}

func personalizeScalar(doc *yaml.Node, value string, path ...string) bool {
	node := manifestNode(doc, path...)
	if node == nil || value == "" {
		return false
	}
	node.Kind, node.Tag, node.Value, node.Style = yaml.ScalarNode, "!!str", value, 0
	node.Content, node.Alias, node.Anchor = nil, nil, ""
	return true
}

// personalizeManifests writes the interview's answers into every manifest just installed.
func personalizeManifests(root, owner, contact, juris string) (int, error) {
	dir := filepath.Join(root, "agentarch", "project", "agents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var jurisdictions []*yaml.Node
	if juris != "" {
		for _, j := range strings.Split(juris, ",") {
			jurisdictions = append(jurisdictions, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str",
				Style: yaml.DoubleQuotedStyle, Value: strings.ToUpper(strings.TrimSpace(j))})
		}
	}

	changed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name(), "agent.yaml")
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return changed, err
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return changed, fmt.Errorf("personalize %s: %w", p, err)
		}
		touched := personalizeScalar(&doc, owner, "agent", "owner", "accountable")
		if personalizeScalar(&doc, contact, "agent", "owner", "contact") {
			touched = true
		}
		if node := manifestNode(&doc, "agent", "jurisdictions"); node != nil {
			node.Kind, node.Tag, node.Value, node.Style = yaml.SequenceNode, "!!seq", "", yaml.FlowStyle
			node.Content, node.Alias, node.Anchor = jurisdictions, nil, ""
			touched = true
		}
		if !touched {
			continue
		}
		var out bytes.Buffer
		encoder := yaml.NewEncoder(&out)
		encoder.SetIndent(2)
		if err := encoder.Encode(&doc); err != nil {
			return changed, fmt.Errorf("personalize %s: %w", p, err)
		}
		if err := encoder.Close(); err != nil {
			return changed, err
		}
		if err := os.WriteFile(p, out.Bytes(), 0o644); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}

// ------------------------------------------------------- the two endings

func finishStartNew(
	root string, bp blueprint.Blueprint, profile string, ownerSet bool, credential string,
) int {
	// Naming the wrong variable in the one command somebody copies is a five-minute detour that
	// ends in "the credential is not set" while the credential is, in fact, set.
	if credential == "" {
		credential = "<PROVIDER>_API_KEY"
	}
	fmt.Printf("\nChecking what you just installed.\n\n")

	if code := cmdValidate([]string{"--root", root}); code != exitOK {
		fmt.Fprintf(os.Stderr, "\nThe installed project does not validate. That is a bug in the\n"+
			"blueprint, not in anything you did — please open an issue.\n")
		return code
	}
	if code := cmdCheck([]string{"--root", root, "--profile", profile}); code != exitOK {
		fmt.Fprintf(os.Stderr, "\nThe gate blocked a project straight out of the box. That is a bug\n"+
			"in the blueprint — please open an issue.\n")
		return code
	}

	fmt.Printf("\n%s\n", strings.Repeat("─", 60))

	// owner.accountable is only named as pending when the blueprint's example person is
	// actually still there. With --owner passed, that sentence would undo the answer.
	ownerLine := `purpose, out_of_scope, and owner.accountable — which still names
     the blueprint's example person. What it must refuse is the part
     that decides everything else.`
	if ownerSet {
		ownerLine = `purpose and out_of_scope. What it must refuse is the part that
     decides everything else.`
	}

	fmt.Printf(`
Done. You have a project that runs and passes its own gate.

Run it:

  python -m venv .venv && source .venv/bin/activate
  pip install -r app/requirements.txt
  export %s=...
  python -m app.cli "%s"

Then make it yours, in this order:

  1. agentarch/project/agents/%s/agent.yaml
     %s
  2. the system prompt beside it — mirror out_of_scope into the
     refusal section, then bump the version and run `+"`agentarch validate`"+`,
     which prints the hash to record.
  3. app/ — replace the placeholder implementations with yours.

  agentarch check          after every change
  agentarch conformance    L1 / L2 / L3, and when it expires
  cat app/README.md        what each file is for

The manifest is the contract. Change it first, then make the code
match: `+"`agentarch check`"+` is what tells you when the two disagree.
`, credential, sampleQuestion(bp), firstAgentID(root), ownerLine)
	return exitOK
}

func finishStartAdopt(root string, ids []string, refactor bool) int {
	id := "your-agent"
	if len(ids) > 0 {
		id = ids[0]
	}

	if refactor {
		// The tool does not rewrite the code. It installs the procedure, and the assistant
		// already reading this project's instruction files carries it out — which is why the
		// checklist matters as much as the skill: the skill reaches Claude, and the checklist
		// reaches everything else.
		fmt.Printf(`
Done. agentarch is installed, there is a manifest describing what is here,
and the refactoring workflow is in place.

agentarch does not rewrite your code. It installs the procedure and then
checks the result — the refactoring itself is done by you, or by whichever
assistant reads this project:

  agentarch/std/checklists/refactor.md      any assistant, any IDE, or by hand
  .claude/skills/agentarch-refactor/        loaded automatically by Claude Code

Start here, in this order:

  1. agentarch check --profile standard --adopt-baseline
     Records today's failures. Without a number to start from there is no
     way to prove later that anything improved.
  2. Open the checklist, or ask your assistant to refactor to the standard.
     It works in verifiable slices: a test for the current behaviour first,
     then the change, then the gate.
  3. agentarch check --profile standard --update-baseline
     Closes what you fixed. The ratchet only turns one way.

The rule the whole procedure enforces: never change behaviour and structure
in the same commit.
`)
		return exitOK
	}

	fmt.Printf(`
Done. agentarch is installed and there is a manifest describing what is
here — with everything the scan could not determine left as "unknown".

That is on purpose. A plausible-looking wrong value is worse than a
blank one, because nobody re-examines a field that is already filled in.

Next, in this order:

  1. agentarch/project/agents/%s/agent.yaml
     Fill in out_of_scope, purpose, and autonomy.level. Describe the
     agent as it is TODAY, not as you would like it to be.
  2. agentarch validate
     Structure only. It will name every field it cannot accept.
  3. agentarch check --adopt-baseline
     Records today's failures as the starting point, so the gate blocks
     only what is new or worse.

Nothing is forgiven by that baseline — `+"`agentarch score`"+` still counts the
debt, and you close it deliberately with --update-baseline.
`, id)
	return exitOK
}

// sampleQuestion is the line to type first. A getting-started flow that ends without a command
// whose output you can see has not actually started anything.
func sampleQuestion(bp blueprint.Blueprint) string {
	switch bp.Meta.ID {
	case "rag-support":
		return "where is my order BR-77120?"
	case "tool-approval":
		return "refund order BR-77120"
	case "multi-agent-handoff":
		return "I was charged twice for order BR-77120"
	case "mcp-consumer":
		return "how do I rotate an API key?"
	case "chatbot-web":
		return "how much does the Pro plan cost?"
	}
	return "hello"
}

func firstAgentID(root string) string {
	entries, err := os.ReadDir(filepath.Join(root, "agentarch", "project", "agents"))
	if err != nil {
		return "<agent>"
	}
	for _, e := range entries {
		if e.IsDir() {
			return e.Name()
		}
	}
	return "<agent>"
}

func reportScan(s adoptScan) {
	if len(s.Providers) == 0 && len(s.Models) == 0 && len(s.Frameworks) == 0 {
		fmt.Printf("found nothing recognisable, so almost every field is `unknown`\n")
		return
	}
	if len(s.Providers) > 0 {
		fmt.Printf("  providers   %s\n", s.withWhere("provider", s.Providers, 5))
	}
	if len(s.Models) > 0 {
		fmt.Printf("  models      %s  (not written to model.id; confirm it yourself)\n", s.withWhere("model", s.Models, 5))
	}
	if len(s.Frameworks) > 0 {
		fmt.Printf("  frameworks  %s\n", s.withWhere("framework", s.Frameworks, 5))
	}
	if len(s.Prompts) > 0 {
		fmt.Printf("  prompts?    %s\n", strings.Join(s.Prompts[:min(3, len(s.Prompts))], ", "))
	}
}
