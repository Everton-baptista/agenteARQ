package main

// `agentarch hook --event <event> --host claude` — the host's real-time guardrail.
//
// Until this existed, everything AgenteARQ asked of a coding assistant during a session lived in
// prompts: "never edit a generated file", "never deploy without the owner". A prompt is guidance;
// the model can drift from it and nothing notices until CI. A host hook is different: the host
// runs this command before the tool call happens and obeys its answer. So the rules that can be
// decided from the tool call alone move here, where they are enforced rather than requested.
//
// What belongs here is deliberately narrow. Every decision is deterministic and made from the
// tool call and the files on disk: no model is called, nothing is sent anywhere, and the answer
// for the same input is always the same. Anything that needs judgement stays in the skills and
// in the gates `agentarch workflow` already enforces. The hook does not replace those gates —
// CI's `agentarch check` remains the guarantee wherever a host does not run hooks (see
// docs/plugins.md for each host's real boundary).
//
// Fail mode: a deterministic guardrail is fail_closed (invariant 12). A pre-tool input this
// command cannot read is denied with a reason rather than allowed. Outside an AgenteARQ project
// the hook allows everything silently: it governs AgenteARQ projects, not the user's machine.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/application"
	"github.com/Everton-baptista/agenteARQ/internal/mcpserver"
	"github.com/Everton-baptista/agenteARQ/internal/render"
	"github.com/Everton-baptista/agenteARQ/internal/validate"
)

// hookEvents are the events the hook answers, named host-neutrally. Each host adapter maps its
// own event names onto these; Claude Code's are SessionStart, PreToolUse and Stop.
var hookEvents = []string{"session-start", "pre-tool", "stop"}

// hookInput is the subset of a host's hook payload the decisions need. Unknown fields are
// ignored, so a host adding fields never breaks the hook.
type hookInput struct {
	Event          string          `json:"hook_event_name"`
	CWD            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	StopHookActive bool            `json:"stop_hook_active"`
	Source         string          `json:"source"`
}

type toolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Content      string `json:"content"`
	NewString    string `json:"new_string"`
	NewSource    string `json:"new_source"`
	Command      string `json:"command"`
	Edits        []struct {
		NewString string `json:"new_string"`
	} `json:"edits"`
}

// decision is a host-neutral verdict. Allow is the zero value.
type decision struct {
	Verdict string // "", "ask" or "deny"
	Reason  string
}

const hookInputLimit = 4 << 20

func cmdHook(args []string) int {
	f := flag.NewFlagSet("hook", flag.ContinueOnError)
	event := f.String("event", "", "session-start, pre-tool or stop")
	host := f.String("host", "claude", "host whose hook protocol to speak (claude or codex)")
	root := f.String("root", "", "project root (default: the hook payload's cwd, searched upward)")
	if err := f.Parse(hoistFlags(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if !slices.Contains(hookEvents, *event) {
		fmt.Fprintf(os.Stderr, "hook: --event must be one of %s\n", strings.Join(hookEvents, ", "))
		return exitUsage
	}
	if *host != "claude" && *host != "codex" {
		fmt.Fprintln(os.Stderr, "hook: the claude and codex hook protocols are implemented; other hosts rely on agentarch check in CI")
		return exitUsage
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, hookInputLimit+1))
	var in hookInput
	if err == nil && len(raw) > hookInputLimit {
		err = errors.New("hook payload exceeds 4 MiB")
	}
	if err == nil && len(bytes.TrimSpace(raw)) > 0 {
		err = json.Unmarshal(raw, &in)
	}
	start := *root
	if start == "" {
		start = in.CWD
	}
	if start == "" {
		start = "."
	}
	project, rootErr := mcpserver.ResolveRoot(start)
	if rootErr != nil {
		// Not an AgenteARQ project: nothing to govern.
		return exitOK
	}
	lang = projectLanguage(project)
	if err != nil {
		if *event == "pre-tool" {
			return emitClaude(os.Stdout, *event, decision{Verdict: "deny", Reason: hookMsg("unreadable") + " (" + err.Error() + ")"}, "")
		}
		return exitOK
	}
	switch *event {
	case "session-start":
		return emitClaude(os.Stdout, *event, decision{}, sessionContext(project))
	case "pre-tool":
		d := preToolDecision(project, in)
		if *host == "codex" && d.Verdict == "ask" {
			// Codex parses "ask" but does not support it: the call would go ahead. Deny
			// instead, so the owner-only action waits for the owner (fail closed).
			d = decision{Verdict: "deny", Reason: d.Reason + " " + hookMsg("codex_no_ask")}
		}
		return emitClaude(os.Stdout, *event, d, "")
	case "stop":
		if in.StopHookActive {
			// The assistant is already continuing because of this hook; blocking again
			// would loop. The reminder was given once, which is what it is for.
			return exitOK
		}
		return emitClaude(os.Stdout, *event, stopDecision(project), "")
	}
	return exitOK
}

// emitClaude writes Claude Code's hook protocol: JSON on stdout with exit 0. An allow with no
// context writes nothing, which Claude Code reads as "proceed".
func emitClaude(w io.Writer, event string, d decision, context string) int {
	var out map[string]any
	switch event {
	case "session-start":
		if context == "" {
			return exitOK
		}
		out = map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "SessionStart", "additionalContext": context}}
	case "pre-tool":
		if d.Verdict == "" {
			return exitOK
		}
		out = map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "permissionDecision": d.Verdict, "permissionDecisionReason": d.Reason}}
	case "stop":
		if d.Verdict == "" {
			if d.Reason == "" {
				return exitOK
			}
			out = map[string]any{"systemMessage": d.Reason}
		} else {
			out = map[string]any{"decision": "block", "reason": d.Reason}
		}
	}
	b, _ := json.Marshal(out)
	fmt.Fprintln(w, string(b))
	return exitOK
}

// sessionContext is the compact, deterministic status injected at the start of every session
// and again after compaction, so the assistant learns where the product stands from recorded
// state rather than from its memory of the conversation. It stays far below the host's cap on
// injected context: a status line, the next step and at most five missing items.
func sessionContext(root string) string {
	v, err := application.Status(root)
	if err != nil {
		return hookMsg("status_error") + " " + err.Error()
	}
	if !v.Initialized || v.Snapshot == nil {
		return hookMsg("not_initialized")
	}
	s := v.Snapshot
	var b strings.Builder
	fmt.Fprintf(&b, hookMsg("status_line"), s.Product.Name, s.Product.Profile, s.State.Phase, s.State.Revision)
	b.WriteString("\n")
	fmt.Fprintf(&b, hookMsg("next_line"), translateAction(s.Next.Action))
	missing := s.Next.Missing
	if len(missing) > 0 {
		b.WriteString("\n" + hookMsg("missing_line"))
		for i, m := range missing {
			if i == 5 {
				fmt.Fprintf(&b, "\n  … +%d", len(missing)-5)
				break
			}
			b.WriteString("\n  - " + translateMissing(m))
		}
	}
	b.WriteString("\n" + hookMsg("rules_line"))
	if lang != render.SourceLang {
		b.WriteString("\n" + fmt.Sprintf(hookMsg("language_line"), lang))
	}
	return b.String()
}

// preToolDecision decides one tool call. File tools are checked for the protected files and for
// secret-shaped content; shell commands for actions that only the owner may authorize.
func preToolDecision(root string, in hookInput) decision {
	var ti toolInput
	if len(in.ToolInput) > 0 {
		if err := json.Unmarshal(in.ToolInput, &ti); err != nil {
			return decision{Verdict: "deny", Reason: hookMsg("unreadable") + " (" + err.Error() + ")"}
		}
	}
	switch in.ToolName {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		path := ti.FilePath
		if path == "" {
			path = ti.NotebookPath
		}
		if d := protectedFileDecision(root, in.CWD, path); d.Verdict != "" {
			return d
		}
		content := ti.Content + "\n" + ti.NewString + "\n" + ti.NewSource
		for _, e := range ti.Edits {
			content += "\n" + e.NewString
		}
		if kind := secretKind(content); kind != "" {
			return decision{Verdict: "ask", Reason: fmt.Sprintf(hookMsg("secret"), kind)}
		}
	case "Bash":
		return commandDecision(ti.Command)
	case "apply_patch":
		// Codex edits files through a patch in tool_input.command: check every file it
		// touches and every line it adds, exactly as an Edit or Write would be checked.
		paths, added := parsePatch(ti.Command)
		for _, path := range paths {
			if d := protectedFileDecision(root, in.CWD, path); d.Verdict != "" {
				return d
			}
		}
		if kind := secretKind(added); kind != "" {
			return decision{Verdict: "ask", Reason: fmt.Sprintf(hookMsg("secret"), kind)}
		}
	}
	return decision{}
}

var patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$`)

// parsePatch reads Codex's apply_patch format: the files it names and the lines it adds.
func parsePatch(patch string) (paths []string, added string) {
	for _, m := range patchFile.FindAllStringSubmatch(patch, -1) {
		paths = append(paths, strings.TrimSpace(m[1]))
	}
	var b strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			b.WriteString(line[1:] + "\n")
		}
	}
	return paths, b.String()
}

// protectedFileDecision denies edits to files no assistant may change by hand: generated
// instruction files (rule 15) and the workflow's own evidence, which only agentarch writes.
func protectedFileDecision(root, cwd, path string) decision {
	if path == "" {
		return decision{}
	}
	if !filepath.IsAbs(path) {
		base := cwd
		if base == "" {
			base = root
		}
		path = filepath.Join(base, path)
	}
	rel, err := filepath.Rel(root, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		slash := filepath.ToSlash(rel)
		if slash == "agentarch/project/evidence/latest.json" || strings.HasPrefix(slash, "agentarch/project/evidence/") {
			return decision{Verdict: "deny", Reason: hookMsg("evidence")}
		}
	}
	if strings.Contains(filepath.ToSlash(path), "/agentarch-workflow/") {
		return decision{Verdict: "deny", Reason: hookMsg("evidence")}
	}
	if isGenerated(path) {
		return decision{Verdict: "deny", Reason: hookMsg("generated")}
	}
	return decision{}
}

// isGenerated reports whether an existing file carries the agentarch:generated header in its
// first lines, which is where every sync target writes it.
func isGenerated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 1024)
	n, _ := io.ReadFull(f, head)
	return bytes.Contains(head[:n], []byte("agentarch:generated"))
}

// secretPatterns recognise credential shapes with a low false-positive rate. A match asks the
// owner rather than denying: test fixtures legitimately contain fake keys, and the owner is the
// one who knows which this is. The security floor's gitleaks scan still runs at verification.
var secretPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"Anthropic API key", regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`)},
	{"OpenAI-style API key", regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_-]{32,}`)},
	{"Google API key", regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}`)},
	{"AWS access key", regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`)},
	{"Stripe live key", regexp.MustCompile(`\b[rs]k_live_[A-Za-z0-9]{20,}`)},
	{"Supabase service_role key", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]*c2VydmljZV9yb2xl[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+`)},
}

func secretKind(s string) string {
	for _, p := range secretPatterns {
		if p.re.MatchString(s) {
			return p.kind
		}
	}
	return ""
}

// ownerOnlyCommands are actions with effect irreversible or communication: they publish, deploy
// or change shared history. The assistant may propose them; the owner approves each one
// (invariant 1). Each entry is a word sequence matched at the start of a command segment.
var ownerOnlyCommands = [][]string{
	{"git", "push"}, {"git", "tag"}, {"git", "merge"},
	{"gh", "pr", "merge"}, {"gh", "pr", "create"}, {"gh", "release", "create"}, {"gh", "repo", "delete"},
	{"vercel", "deploy"}, {"vercel", "--prod"}, {"netlify", "deploy"}, {"fly", "deploy"}, {"flyctl", "deploy"},
	{"railway", "up"}, {"wrangler", "deploy"}, {"wrangler", "publish"}, {"wrangler", "pages", "deploy"},
	{"firebase", "deploy"}, {"heroku"}, {"kubectl", "apply"}, {"kubectl", "delete"},
	{"terraform", "apply"}, {"terraform", "destroy"}, {"pulumi", "up"}, {"supabase", "db", "push"},
	{"npm", "publish"}, {"pnpm", "publish"}, {"yarn", "publish"}, {"cargo", "publish"}, {"twine", "upload"},
	{"docker", "push"}, {"podman", "push"},
}

var segmentSplit = regexp.MustCompile(`&&|\|\||;|\||\n|\$\(|` + "`")
var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
var quotedText = regexp.MustCompile(`'[^']*'|"(?:[^"\\$]|\\.)*"`)

// commandDecision inspects a shell command segment by segment. It is not a shell parser and does
// not pretend to be one: it recognises the plain forms an assistant actually writes. Evasion by
// obfuscation is out of scope here and is what CI's gate and the sandbox are for.
func commandDecision(command string) decision {
	if strings.Contains(command, "--no-verify") {
		return decision{Verdict: "ask", Reason: hookMsg("no_verify")}
	}
	writesEvidence := regexp.MustCompile(`(>|\btee\b|\bsed\s+-i|\brm\b|\bmv\b|\bcp\b|\btruncate\b)`)
	if (strings.Contains(command, "agentarch-workflow/") || strings.Contains(command, "agentarch/project/evidence/")) && writesEvidence.MatchString(command) {
		return decision{Verdict: "deny", Reason: hookMsg("evidence")}
	}
	// Quoted text is data (a commit message, an echo), never a command word.
	unquoted := quotedText.ReplaceAllString(command, " ")
	for _, segment := range segmentSplit.Split(unquoted, -1) {
		words := strings.Fields(strings.TrimSpace(segment))
		for i, w := range words {
			words[i] = strings.Trim(w, "()'\"{}")
		}
		for len(words) > 0 && (envAssignment.MatchString(words[0]) || words[0] == "sudo" || words[0] == "command" || words[0] == "exec" || words[0] == "npx" || words[0] == "env") {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		for _, cmd := range ownerOnlyCommands {
			if matchesWords(words, cmd) {
				return decision{Verdict: "ask", Reason: fmt.Sprintf(hookMsg("owner_only"), strings.Join(cmd, " "))}
			}
		}
	}
	return decision{}
}

// matchesWords matches a command's leading word (by base name, so /usr/bin/git counts) and the
// remaining pattern words anywhere after it, so `git -C dir push` is still a push.
func matchesWords(words, pattern []string) bool {
	if filepath.Base(words[0]) != pattern[0] {
		return false
	}
	rest := words[1:]
	for _, p := range pattern[1:] {
		found := false
		for i, w := range rest {
			if w == p {
				rest, found = rest[i+1:], true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// stopDecision runs when the assistant is about to end its turn. It blocks once when an agent
// artifact the assistant may have changed no longer validates, and otherwise reminds — without
// blocking — when the recorded verification no longer matches the source.
func stopDecision(root string) decision {
	if entries, err := os.ReadDir(filepath.Join(root, "agentarch", "project", "agents")); err == nil && len(entries) > 0 {
		if v, err := validate.New(agentarch.Spec); err == nil {
			if findings, err := v.Project(root); err == nil && len(findings) > 0 {
				var lines []string
				for i, f := range findings {
					if i == 5 {
						lines = append(lines, fmt.Sprintf("… +%d", len(findings)-5))
						break
					}
					lines = append(lines, f.String())
				}
				return decision{Verdict: "block", Reason: hookMsg("validate_failed") + "\n" + strings.Join(lines, "\n")}
			}
		}
	}
	v, err := application.Status(root)
	if err != nil || v.Snapshot == nil {
		return decision{}
	}
	if s := v.Snapshot; s.State.Phase == "verifying" && s.State.Verification != nil {
		return decision{Reason: hookMsg("verify_stale")}
	}
	return decision{}
}

// hookMessages are short and actionable: each says what happened and what to do instead.
var hookMessages = map[string]map[string]string{
	"en": {
		"unreadable":      "AgenteARQ could not read this tool call, so it is blocked (fail closed).",
		"status_error":    "AgenteARQ could not read the workflow state:",
		"not_initialized": "AgenteARQ is installed here but no product workflow exists yet. Ask the owner what they want to build, then run `agentarch start` (or `agentarch workflow init`).",
		"status_line":     "AgenteARQ — product %s (profile %s): phase %s, revision %d.",
		"next_line":       "Next step: %s",
		"missing_line":    "Still missing:",
		"rules_line":      "Rules: one owner decision at a time, with options and a labelled recommendation; never edit generated files or evidence; publishing, merging and deploying need the owner's approval; run `agentarch workflow status` instead of guessing the state.",
		"generated":       "This file is generated by agentarch (agentarch:generated). Edit its source under agentarch/std/core/ and run `agentarch sync` (rule 15).",
		"evidence":        "Workflow state and evidence are written only by agentarch commands. Use `agentarch workflow checkpoint`, `verify` or `review` instead of editing them.",
		"secret":          "This content looks like a %s. Secrets are referenced by name (an environment variable), never written into files (invariant 3). Approve only if this is a known fake test value.",
		"owner_only":      "`%s` publishes, deploys or changes shared history. AgenteARQ never does this on its own: the owner must approve it.",
		"no_verify":       "--no-verify skips the project's own checks. The owner must approve skipping them.",
		"validate_failed": "Agent artifacts no longer validate. Fix them (or tell the owner why not) before finishing:",
		"language_line":   "The owner's language is %s: speak to them and write every document in it.",
		"codex_no_ask":    "Codex cannot ask for approval from a hook, so AgenteARQ blocks it: the owner runs it, or approves it outside the assistant.",
		"verify_stale":    "AgenteARQ: there is no passing verification for the current source. Run `agentarch workflow verify` before claiming the work is done.",
	},
	"pt-BR": {
		"unreadable":      "O AgenteARQ não conseguiu ler esta chamada de ferramenta, então ela foi bloqueada (fail closed).",
		"status_error":    "O AgenteARQ não conseguiu ler o estado do workflow:",
		"not_initialized": "O AgenteARQ está instalado aqui, mas ainda não existe workflow de produto. Pergunte ao dono o que ele quer construir e rode `agentarch start` (ou `agentarch workflow init`).",
		"status_line":     "AgenteARQ — produto %s (perfil %s): fase %s, revisão %d.",
		"next_line":       "Próximo passo: %s",
		"missing_line":    "Ainda falta:",
		"rules_line":      "Regras: uma decisão do dono por vez, com opções e uma recomendação identificada; nunca edite arquivos gerados nem evidências; publicar, fazer merge e deploy exigem aprovação do dono; rode `agentarch workflow status` em vez de supor o estado.",
		"generated":       "Este arquivo é gerado pelo agentarch (agentarch:generated). Edite a fonte em agentarch/std/core/ e rode `agentarch sync` (regra 15).",
		"evidence":        "Estado e evidências do workflow só são escritos por comandos do agentarch. Use `agentarch workflow checkpoint`, `verify` ou `review` em vez de editá-los.",
		"secret":          "Este conteúdo parece ser um(a) %s. Segredos são referenciados pelo nome (variável de ambiente), nunca escritos em arquivos (invariante 3). Aprove só se for um valor falso de teste conhecido.",
		"owner_only":      "`%s` publica, faz deploy ou altera o histórico compartilhado. O AgenteARQ nunca faz isso sozinho: o dono precisa aprovar.",
		"no_verify":       "--no-verify pula as verificações do próprio projeto. O dono precisa aprovar.",
		"validate_failed": "Os artefatos de agente não validam mais. Corrija-os (ou explique ao dono por quê) antes de encerrar:",
		"language_line":   "O idioma do dono é %s: fale com ele e escreva todos os documentos nesse idioma.",
		"codex_no_ask":    "O Codex não consegue pedir aprovação a partir de um hook, então o AgenteARQ bloqueia: o dono executa, ou aprova fora do assistente.",
		"verify_stale":    "AgenteARQ: não há verificação aprovada para o código atual. Rode `agentarch workflow verify` antes de dizer que o trabalho está pronto.",
	},
}

func hookMsg(key string) string {
	if m, ok := hookMessages[lang][key]; ok {
		return m
	}
	return hookMessages["en"][key]
}
