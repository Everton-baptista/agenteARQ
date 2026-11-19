package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

// adoptScan is what could be inferred from a project that already has agents in it.
type adoptScan struct {
	Providers  []string
	Models     []string
	Prompts    []string
	Frameworks []string
	MCPFiles   []string
	// Where reports the first file each provider, model and framework was seen in, so a person
	// can check a finding instead of taking it on trust.
	Where map[string]string
}

// pkgHint maps a package as a project declares or imports it to what it implies.
type pkgHint struct{ kind, value string }

var (
	// Model identifiers, deliberately narrow. A wrong guess that looks confident is worse than
	// a field left `unknown`, because nobody re-examines a field that is already filled in.
	modelRe = regexp.MustCompile(`["']((?:claude|gpt|gemini|llama|mistral|command|deepseek|qwen)(?:-|\d)[a-zA-Z0-9._:-]{0,60})["']`)

	// Only declared dependencies and imports count as evidence. Matching substrings anywhere
	// reported "agno" for every file that said "diagnostic" and the OpenAI Agents SDK for every
	// file that said "agents".
	packages = map[string]pkgHint{
		"anthropic": {"provider", "anthropic"}, "@anthropic-ai/sdk": {"provider", "anthropic"},
		"github.com/anthropics/anthropic-sdk-go": {"provider", "anthropic"},
		"openai":                                 {"provider", "openai"}, "github.com/openai/openai-go": {"provider", "openai"},
		"google-generativeai": {"provider", "google"}, "google-genai": {"provider", "google"},
		"@google/genai": {"provider", "google"}, "@google/generative-ai": {"provider", "google"},
		"google.golang.org/genai": {"provider", "google"}, "google-cloud-aiplatform": {"provider", "google"},
		"ollama":    {"provider", "local"},
		"langgraph": {"framework", "langgraph"}, "@langchain/langgraph": {"framework", "langgraph"},
		"langchain":   {"framework", "langgraph"},
		"llama-index": {"framework", "llamaindex"}, "llamaindex": {"framework", "llamaindex"},
		"crewai": {"framework", "crewai"}, "agno": {"framework", "agno"},
		"semantic-kernel": {"framework", "semantic-kernel"}, "pydantic-ai": {"framework", "pydantic-ai"},
		"openai-agents": {"framework", "openai-agents-sdk"}, "@openai/agents": {"framework", "openai-agents-sdk"},
		"claude-agent-sdk":               {"framework", "claude-agent-sdk"},
		"@anthropic-ai/claude-agent-sdk": {"framework", "claude-agent-sdk"},
		"google-adk":                     {"framework", "google-adk"}, "ai": {"framework", "vercel-ai-sdk"},
	}

	// Python imports by top-level module. `agents` is left out: it is the OpenAI Agents SDK's
	// module name and also the most common name for a project's own package.
	pyModules = map[string]string{
		"anthropic": "anthropic", "openai": "openai", "google.generativeai": "google-generativeai",
		"google.genai": "google-genai", "vertexai": "google-cloud-aiplatform", "ollama": "ollama",
		"langgraph": "langgraph", "langchain": "langchain", "llama_index": "llama-index",
		"crewai": "crewai", "agno": "agno", "semantic_kernel": "semantic-kernel",
		"pydantic_ai": "pydantic-ai", "claude_agent_sdk": "claude-agent-sdk", "google.adk": "google-adk",
	}
	pyImportRe = regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([A-Za-z_][\w.]*)`)
	goImportRe = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:[A-Za-z_.]+\s+)?"([a-z0-9.-]+\.[a-z]+/[^"]+)"\s*$`)
	jsImportRe = regexp.MustCompile(`(?:\bfrom\s+|\brequire\(\s*|\bimport\(\s*|^\s*import\s+)["']([^"']+)["']`)

	manifestFiles = map[string]bool{
		"package.json": true, "pyproject.toml": true, "Pipfile": true, "go.mod": true,
		"setup.py": true, "setup.cfg": true, "environment.yml": true,
	}

	skipDirs = map[string]bool{
		".git": true, "node_modules": true, ".venv": true, "venv": true, "site-packages": true,
		"vendor": true, "dist": true, "build": true, "out": true, "target": true, "coverage": true,
		"__pycache__": true, "agentarch": true, ".claude": true,
	}
)

// isRequirements matches requirements.txt and variants such as requirements-dev.txt.
func isRequirements(name string) bool {
	return strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt")
}

// declared reports the packages a dependency manifest names, as whole package names.
func declared(name, body string) []string {
	var out []string
	for pkg := range packages {
		var re *regexp.Regexp
		q := regexp.QuoteMeta(pkg)
		switch {
		case name == "package.json":
			re = regexp.MustCompile(`"` + q + `"\s*:`)
		case name == "go.mod":
			re = regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + q + `(?:/v\d+)?\s+v`)
		case isRequirements(name):
			re = regexp.MustCompile(`(?mi)^\s*` + q + `\s*(?:\[[^\]]*\])?\s*(?:[=<>~!;@]|$)`)
		default: // pyproject, Pipfile, setup.*, environment.yml: a quoted or line-leading name
			re = regexp.MustCompile(`(?mi)(?:^|["'\s,\[])` + q + `\s*(?:\[[^\]]*\])?\s*(?:[=<>~!;"',\]]|$)`)
		}
		if pkg == "ai" && name != "package.json" {
			continue // too short to trust outside package.json
		}
		if re.MatchString(body) {
			out = append(out, pkg)
		}
	}
	return out
}

// imported reports the known packages a source file imports.
func imported(ext, body string) []string {
	var out []string
	switch ext {
	case ".py":
		for _, m := range pyImportRe.FindAllStringSubmatch(body, -1) {
			mod := m[1]
			for prefix, pkg := range pyModules {
				if mod == prefix || strings.HasPrefix(mod, prefix+".") {
					out = append(out, pkg)
				}
			}
		}
	case ".go":
		for _, m := range goImportRe.FindAllStringSubmatch(body, -1) {
			for pkg := range packages {
				if strings.Contains(pkg, ".") && strings.Contains(pkg, "/") && (m[1] == pkg || strings.HasPrefix(m[1], pkg+"/")) {
					out = append(out, pkg)
				}
			}
		}
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		for _, m := range jsImportRe.FindAllStringSubmatch(body, -1) {
			spec := m[1]
			for pkg := range packages {
				if spec == pkg || strings.HasPrefix(spec, pkg+"/") {
					out = append(out, pkg)
				}
			}
		}
	}
	return out
}

// scanForAgents looks for evidence of an existing agent.
//
// It reports what it found and never guesses beyond it. Everything it cannot determine is left
// as `unknown` in the manifest, because a plausible-looking wrong value is worse than a blank
// one: nobody re-examines a field that is already filled in.
func scanForAgents(root string) adoptScan {
	s := adoptScan{Where: map[string]string{}}
	seen := map[string]bool{}
	add := func(dst *[]string, kind, v, rel string) {
		if v == "" || seen[kind+":"+v] {
			return
		}
		seen[kind+":"+v] = true
		*dst = append(*dst, v)
		if kind != "prompt" && kind != "mcp" {
			s.Where[kind+":"+v] = rel
		}
	}
	hint := func(pkg, rel string) {
		h := packages[pkg]
		if h.kind == "provider" {
			add(&s.Providers, "provider", h.value, rel)
		} else {
			add(&s.Frameworks, "framework", h.value, rel)
		}
	}

	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			// A virtual environment under any name holds other people's code, full of model names.
			if _, e := os.Stat(filepath.Join(p, "pyvenv.cfg")); e == nil {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		ext := filepath.Ext(name)
		manifest := manifestFiles[name] || isRequirements(name)
		mcp := name == ".mcp.json" || strings.Contains(name, "mcp") && ext == ".json"
		source := false
		switch ext {
		case ".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".go", ".rb", ".java", ".cs", ".yaml", ".yml", ".toml":
			source = !strings.HasSuffix(name, "-lock.yaml") && !strings.HasSuffix(name, ".lock.yaml") && !strings.HasSuffix(name, ".min.js")
		}
		if !manifest && !source && !mcp {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if mcp {
			add(&s.MCPFiles, "mcp", rel, rel)
		}

		info, err := d.Info()
		if err != nil || info.Size() > 512*1024 {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		body := string(raw)

		if manifest {
			for _, pkg := range declared(name, body) {
				hint(pkg, rel)
			}
		}
		if !source {
			return nil
		}
		imports := imported(ext, body)
		for _, pkg := range imports {
			hint(pkg, rel)
		}
		// Model names and prompts count only where a model is plausibly called: a file that
		// imports a model library, or a configuration file outside interface copy. A dropdown
		// label, a pricing page or a "you are logged in" banner is not evidence of an agent.
		config := (ext == ".yaml" || ext == ".yml" || ext == ".toml") && !inUIDir(rel)
		if len(imports) == 0 && !config {
			return nil
		}
		for _, m := range modelRe.FindAllStringSubmatch(body, -1) {
			add(&s.Models, "model", m[1], rel)
		}
		// A long triple-quoted string mentioning "you are" is usually a system prompt.
		if len(imports) > 0 && strings.Contains(strings.ToLower(body), "you are ") && len(body) > 200 {
			add(&s.Prompts, "prompt", rel, rel)
		}
		return nil
	})

	// A model has to belong to a provider the project actually uses. "gpt-4.1" in a project that
	// only declares anthropic is a comparison table or a leftover, not what runs.
	providers := map[string]bool{}
	for _, p := range s.Providers {
		providers[p] = true
	}
	kept := s.Models[:0]
	for _, m := range s.Models {
		if want, known := modelProvider(m); known && !providers[want] {
			delete(s.Where, "model:"+m)
			continue
		}
		if len(s.Providers) == 0 && len(s.Frameworks) == 0 {
			delete(s.Where, "model:"+m)
			continue
		}
		kept = append(kept, m)
	}
	s.Models = kept

	sort.Strings(s.Providers)
	sort.Strings(s.Models)
	sort.Strings(s.Frameworks)
	sort.Strings(s.Prompts)
	sort.Strings(s.MCPFiles)
	return s
}

// IsAgent reports whether the project calls a model at all: a declared or imported model SDK or
// agent framework. Without one there is no agent to describe, and writing a manifest for it
// would be the confident wrong guess the scan exists to avoid.
func (s adoptScan) IsAgent() bool { return len(s.Providers)+len(s.Frameworks) > 0 }

// modelProvider names the provider a model family belongs to, when the family has exactly one.
func modelProvider(model string) (string, bool) {
	switch {
	case strings.HasPrefix(model, "claude"):
		return "anthropic", true
	case strings.HasPrefix(model, "gpt"):
		return "openai", true
	case strings.HasPrefix(model, "gemini"):
		return "google", true
	}
	return "", false // open-weight families run on many providers
}

var uiDirs = map[string]bool{
	"locales": true, "locale": true, "i18n": true, "translations": true, "lang": true,
	"public": true, "static": true, "assets": true, "content": true, "docs": true,
}

func inUIDir(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(rel)), "/") {
		if uiDirs[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

// withWhere renders found values with the file each was first seen in.
func (s adoptScan) withWhere(kind string, values []string, limit int) string {
	if len(values) > limit {
		values = values[:limit]
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = v
		if w := s.Where[kind+":"+v]; w != "" {
			out[i] = fmt.Sprintf("%s (%s)", v, w)
		}
	}
	return strings.Join(out, ", ")
}

// writeAdoptedManifest produces a manifest that is honest about what is not yet known.
func writeAdoptedManifest(root, id string, s adoptScan) error {
	dir := filepath.Join(root, "agentarch", "project", "agents", id)
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		return err
	}

	// A provider is filled in only when exactly one is declared or imported. A model never is:
	// a string in the code says what someone typed, not what runs in production.
	provider, model := "unknown", "unknown"
	if len(s.Providers) == 1 {
		provider = s.Providers[0]
	}

	var found strings.Builder
	if len(s.Providers) > 0 {
		fmt.Fprintf(&found, "#   providers seen:  %s\n", s.withWhere("provider", s.Providers, 5))
	}
	if len(s.Models) > 0 {
		fmt.Fprintf(&found, "#   models seen:     %s  (confirm model.id yourself)\n", s.withWhere("model", s.Models, 5))
	}
	if len(s.Frameworks) > 0 {
		fmt.Fprintf(&found, "#   frameworks seen: %s  (see agentarch/std/adapters/)\n",
			s.withWhere("framework", s.Frameworks, 5))
	}
	if len(s.Prompts) > 0 {
		shown := s.Prompts
		if len(shown) > 3 {
			shown = shown[:3]
		}
		fmt.Fprintf(&found, "#   possible prompts: %s\n", strings.Join(shown, ", "))
	}
	if len(s.MCPFiles) > 0 {
		fmt.Fprintf(&found, "#   MCP config:      %s\n", strings.Join(s.MCPFiles, ", "))
	}
	if found.Len() == 0 {
		found.WriteString("#   nothing recognisable — fill this in from what you know.\n")
	}

	body := fmt.Sprintf(`# Adopted by "agentarch init --adopt".
#
# This describes an agent that already exists. Every field marked "unknown" is something the
# scan could not determine, left blank on purpose: a plausible-looking wrong value is worse
# than a blank one, because nobody re-examines a field that is already filled in.
#
# What the scan found:
%s#
# Work through the unknowns in this order — each answer constrains the next:
#   1. owner.accountable   who would be paged
#   2. out_of_scope        what it must refuse
#   3. autonomy.level      how far it goes unattended today, not how far you wish it did
#   4. model.id            the exact identifier in production
#
# Then run: agentarch check --adopt-baseline
# That records today's failures so the gate blocks only what is new, and you close them
# deliberately instead of all at once.

schema_version: "1.0"
agent:
  id: %s
  name: %s

  owner:
    team: unknown
    contact: unknown
    accountable: unknown      # a person, not a team

  stage: internal             # be honest; claiming production makes the gate stricter
  system_type: agentic_task
  purpose: unknown

  out_of_scope:
    - unknown                 # the three things you would be most alarmed to find it had done

  users:
    audience: internal
    minors_likely: false

  jurisdictions: %s
  languages: ["en"]

  autonomy:
    level: L1_act_with_approval   # what it does TODAY
    max_steps: 10
    max_tool_calls: 20
    stop_conditions:
      - unknown
    budget:
      usd_per_run: 1.0            # a number you would want to be alerted on
      tokens_per_run: 50000
      latency_p95_ms: 15000

  model:
    provider: %s
    id: %s
    pinned: false               # set true once the id is exact; a floating alias changes
                                # behaviour with nothing in the repository to review
    params: {}

  prompts:
    system:
      path: prompts/system.v1.md
      version: 1.0.0
      sha256: unknown           # copy your prompt into the path above, then run
                                # "agentarch validate" — it prints the hash it expected

  guardrails:
    # All three keys must be present. Use an empty list where you genuinely have no check
    # today — that is an honest record of the current state, and the score will show it.
    input: []
    output: []
    action: []

  privacy:
    processes_personal_data: true   # true if it ever sees a name, email, order or ticket
    data_categories: [unknown]
    retention_days: 90
    capture_content: false

  observability:
    otel:
      enabled: false
      capture_content: false

  policy:
    packs: []
    waivers_ref: ../../waivers.yaml

  lifecycle:
    revalidate_on:
      - model_changed
      - system_prompt_changed
      - autonomy_raised
      - guardrail_disabled
    review_interval_days: 90

  links:
    threat_model: threat-model.md
`, found.String(), id, id, "[]", provider, model)

	path := filepath.Join(dir, "agent.yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; adopt would overwrite it", path)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}

	prompt := filepath.Join(dir, "prompts", "system.v1.md")
	if _, err := os.Stat(prompt); os.IsNotExist(err) {
		note := "<!-- Paste the system prompt this agent actually uses today, unchanged.\n" +
			"     Improve it afterwards — adopting and rewriting at the same time makes it\n" +
			"     impossible to tell which change caused which effect.\n\n" +
			"     Then run `agentarch validate`: it will print the hash to record. -->\n"
		if err := os.WriteFile(prompt, []byte(note), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// profileOption is one engineering profile a non-agent product could be verified with.
type profileOption struct {
	ID, Checks, Fits string
	Matches          bool
}

// productProfileOptions lists every available profile with what it checks and what it suits,
// marking the ones the project's files point to. It proposes; the person chooses. Profiles this
// build does not ship are left out rather than suggested.
func productProfileOptions(root string) []profileOption {
	has := func(names ...string) bool {
		for _, n := range names {
			if _, e := os.Stat(filepath.Join(root, n)); e == nil {
				return true
			}
		}
		return false
	}
	count := map[string]int{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		count[strings.ToLower(filepath.Ext(d.Name()))]++
		return nil
	})
	ts := has("tsconfig.json") || count[".ts"]+count[".tsx"] > 0
	code := count[".html"] + count[".py"] + count[".go"] + count[".ts"] + count[".js"]
	matches := map[string]bool{
		"typescript-web": has("package.json") && ts,
		"static-site":    count[".html"] > 0 && !ts,
		"python-api":     has("pyproject.toml", "requirements.txt", "setup.py"),
		"go-service":     has("go.mod"),
		"docs":           count[".md"] > 0 && count[".md"] >= code,
	}
	var out []profileOption
	for _, p := range workflow.ProfileOptions() {
		out = append(out, profileOption{ID: p.ID, Fits: p.Fits, Checks: p.Checks, Matches: matches[p.ID]})
	}
	return out
}

// reportNotAnAgent says plainly that no agent was found and lays out the product workflow's
// profiles side by side, instead of writing a manifest for an agent that does not exist.
func reportNotAnAgent(root string) {
	fmt.Printf(`
No agent here: no model SDK or agent framework is declared or imported, so no
agent.yaml was written. That is fine. The standards under agentarch/std apply
to any product, and the product workflow takes it from spec to production with
isolated verification. Every profile adds the same security floor (secrets and
known-vulnerable dependencies). Pick the one that fits:

`)
	marked := false
	for _, o := range productProfileOptions(root) {
		mark := " "
		if o.Matches {
			mark, marked = "*", true
		}
		fmt.Printf("  %s %-15s %s\n      checks: %s\n", mark, o.ID, o.Fits, o.Checks)
	}
	if marked {
		fmt.Printf("\n  * = what this project's files suggest; nothing is chosen for you.\n")
	}
	fmt.Printf(`
Then:  agentarch workflow init --name <product> --profile <one of the above> \
         --goal "<the user outcome>" --author "<your name>"

If this project does call a model in a way the scan cannot see (a raw HTTP
call, say), describe it with:  agentarch init --adopt --adopt-id <id> --agent
`)
}
