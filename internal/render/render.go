// Package render turns the L0 core into the instruction file each AI assistant expects.
//
// The flow is strictly one-way: core files in, shims out. Nothing here ever reads a generated
// shim as input. That is not an implementation detail — bidirectional sync is precisely what
// makes tools in this space drift, because once both sides can be authoritative there is no
// answer to "which one was right".
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/i18n"
)

// Target is one assistant's instruction file.
type Target struct {
	Name string // identifier used in agentarch.yaml
	Path string // where it is written, relative to the project root
	// Budget is the maximum rendered size in bytes. Exceeding it is a hard failure rather
	// than a silent truncation: an assistant that receives half a rulebook follows half the
	// rules, and nobody finds out until something goes wrong in production.
	Budget int
	// Header wraps the generated block for formats that need front matter (Cursor MDC,
	// Windsurf). Empty means plain markdown.
	FrontMatter string
	// Covers documents which assistants read this file, so the generated header can say so.
	Covers string
}

// Targets is the registry. Adding one is an RFC-level change: every target is a promise that
// the standard keeps working in that tool.
var Targets = []Target{
	{
		Name: "agents_md", Path: "AGENTS.md", Budget: 12288,
		Covers: "the AGENTS.md convention — Codex, Cursor agent mode, Gemini CLI, Grok, Kimi, opencode, Amp, Zed, Aider, Jules",
	},
	{Name: "claude", Path: "CLAUDE.md", Budget: 12288, Covers: "Claude Code"},
	{Name: "gemini", Path: "GEMINI.md", Budget: 12288, Covers: "Gemini CLI"},
	{Name: "qwen", Path: "QWEN.md", Budget: 12288, Covers: "Qwen Code"},
	{
		Name: "cursor", Path: ".cursor/rules/agentarch-core.mdc", Budget: 8192,
		FrontMatter: "---\ndescription: Agent architecture rules (agentarch)\nalwaysApply: true\n---\n",
		Covers:      "Cursor",
	},
	{Name: "copilot", Path: ".github/copilot-instructions.md", Budget: 6144, Covers: "GitHub Copilot"},
	// Derived from agentarch/project/mcp/allowlist.yaml rather than from the core, so the
	// reviewed document is the source and the runtime config is the derivative. Handled
	// specially by sync; the budget is nominal.
	{Name: "mcp_json", Path: ".mcp.json", Budget: 1 << 20, Covers: "any MCP client reading .mcp.json"},
	// Copied from std/skills/ rather than rendered from the core. The same workflows exist as
	// checklists for assistants that do not load skills — a standard that works better in one
	// tool is not a standard.
	{Name: "skills", Path: ".claude/skills", Budget: 1 << 20, Covers: "Claude Code skills"},
	{
		Name: "windsurf", Path: ".windsurf/rules/agentarch-core.md", Budget: 6144,
		FrontMatter: "---\ntrigger: always_on\n---\n",
		Covers:      "Windsurf",
	},
}

// TargetByName returns a target from the registry.
func TargetByName(name string) (Target, bool) {
	for _, t := range Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

// Core is the concatenated L0 block plus the digest that ties every shim back to it.
type Core struct {
	Text   string
	SHA256 string
	Lang   string
	Files  []string
}

// BudgetCore is the ceiling on the concatenated core, in bytes. It is a fixed budget rather
// than a guideline: adding an invariant must cost something, or the core grows until
// assistants start ignoring it. When this fails, the fix is to remove a rule or demote it to a
// standard — not to raise the number.
const BudgetCore = 12288

// ContentVersion reads content/VERSION, the version of the standard's content.
//
// This is what a generated file's header names — not the CLI version. A shim is rendered from
// content, so a header carrying the binary's version would differ between two people running
// the same standard, and `sync --check` would report drift that has nothing to do with the
// rules having changed.
func ContentVersion(fsys fs.FS) string {
	raw, err := fs.ReadFile(fsys, "VERSION")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(raw))
}

// BuildCore reads core/<lang>/*.md in filename order and concatenates it.
//
// fsys is rooted at the content tree itself, so the embedded payload and a project's installed
// agentarch/std look identical here. Callers do the rooting; letting this function know which
// of the two it was handed is what made the paths diverge in the first place.
func BuildCore(fsys fs.FS, lang string) (Core, error) {
	dir := path.Join("core", lang)
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return Core{}, fmt.Errorf("core for language %q not found: %w", lang, err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return Core{}, fmt.Errorf("no core files in %s", dir)
	}
	sort.Strings(names)

	var b strings.Builder
	for i, n := range names {
		raw, err := fs.ReadFile(fsys, path.Join(dir, n))
		if err != nil {
			return Core{}, err
		}
		// Translation front matter is bookkeeping for `validate`, not something an
		// assistant should read. Leaving it in would spend context budget on a hash and
		// invite the model to reason about provenance instead of about the rules.
		_, body, hasFM := i18n.ParseFrontMatter(string(raw))
		if !hasFM {
			body = string(raw)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimLeft(body, "\n"))
	}

	text := b.String()
	sum := sha256.Sum256([]byte(text))
	return Core{Text: text, SHA256: hex.EncodeToString(sum[:]), Lang: lang, Files: names}, nil
}

var (
	headerRe = regexp.MustCompile(`(?m)^<!-- agentarch:generated .*-->\n?`)
	// customRe captures a user's own additions. Without an officially supported escape
	// hatch, the first legitimate customisation turns into a fight with the tool, and the
	// team's resolution is to stop running sync — which loses everything else too.
	customRe = regexp.MustCompile(`(?s)<!-- agentarch:custom:start -->(.*?)<!-- agentarch:custom:end -->`)
)

// ExtractCustom returns the custom regions found in an existing file, in order.
func ExtractCustom(existing string) []string {
	var out []string
	for _, m := range customRe.FindAllStringSubmatch(existing, -1) {
		out = append(out, m[1])
	}
	return out
}

// Render produces the full file content for one target.
//
// version identifies the content release; existing is the file currently on disk, if any, and
// is read only to carry custom regions forward.
func Render(t Target, core Core, version, existing string) (string, error) {
	var b strings.Builder

	b.WriteString(t.FrontMatter)
	fmt.Fprintf(&b,
		"<!-- agentarch:generated v=%s core_sha256=%s target=%s lang=%s\n"+
			"     DO NOT EDIT. Edit agentarch/std/core/%s/ and run `agentarch sync`.\n"+
			"     Read by: %s -->\n\n",
		version, core.SHA256, t.Name, core.Lang, core.Lang, t.Covers)

	b.WriteString(core.Text)

	if custom := ExtractCustom(existing); len(custom) > 0 {
		b.WriteString("\n\n<!-- agentarch:custom:start -->")
		b.WriteString(strings.Join(custom, "\n"))
		b.WriteString("<!-- agentarch:custom:end -->\n")
	}

	out := b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}

	if len(out) > t.Budget {
		return "", fmt.Errorf(
			"target %s renders to %d bytes, over its %d byte budget by %d — "+
				"remove or demote a core rule rather than raising the budget",
			t.Name, len(out), t.Budget, len(out)-t.Budget)
	}
	return out, nil
}

// CoreSHAOf reads the core_sha256 recorded in a generated file. Empty when absent, which is
// how a hand-written or foreign file is told apart from a stale generated one.
func CoreSHAOf(content string) string {
	m := regexp.MustCompile(`core_sha256=([a-f0-9]{64})`).FindStringSubmatch(content)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

// StripHeader removes the generated header, for diffing purposes.
func StripHeader(s string) string { return headerRe.ReplaceAllString(s, "") }
