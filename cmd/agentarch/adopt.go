package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// adoptScan is what could be inferred from a project that already has agents in it.
type adoptScan struct {
	Providers  []string
	Models     []string
	Prompts    []string
	Frameworks []string
	MCPFiles   []string
}

var (
	// Model identifiers, deliberately narrow. A wrong guess that looks confident is worse than
	// a field left `unknown`, because nobody re-examines a field that is already filled in.
	modelRe = regexp.MustCompile(`["']((?:claude|gpt|gemini|llama|mistral|command|deepseek|qwen)[a-zA-Z0-9._-]{2,60})["']`)

	providerHints = map[string]string{
		"anthropic":           "anthropic",
		"openai":              "openai",
		"google.generativeai": "google",
		"google-genai":        "google",
		"bedrock":             "bedrock",
		"boto3":               "bedrock",
		"vertexai":            "google",
		"ollama":              "local",
	}

	frameworkHints = map[string]string{
		"langgraph": "langgraph", "langchain": "langgraph",
		"llama_index": "llamaindex", "llamaindex": "llamaindex",
		"crewai": "crewai", "agno": "agno",
		"semantic_kernel":  "semantic-kernel",
		"pydantic_ai":      "pydantic-ai",
		"agents":           "openai-agents-sdk",
		"claude_agent_sdk": "claude-agent-sdk",
		"google.adk":       "google-adk",
		"ai/react":         "vercel-ai-sdk",
	}

	skipDirs = map[string]bool{
		".git": true, "node_modules": true, ".venv": true, "venv": true,
		"vendor": true, "dist": true, "build": true, "__pycache__": true,
		"agentarch": true, ".claude": true,
	}
)

// scanForAgents looks for evidence of an existing agent.
//
// It reports what it found and never guesses beyond it. Everything it cannot determine is left
// as `unknown` in the manifest, because a plausible-looking wrong value is worse than a blank
// one: nobody re-examines a field that is already filled in.
func scanForAgents(root string) adoptScan {
	var s adoptScan
	seen := map[string]bool{}
	add := func(dst *[]string, v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		*dst = append(*dst, v)
	}

	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		ext := filepath.Ext(name)
		switch ext {
		case ".py", ".ts", ".js", ".go", ".rb", ".java", ".cs", ".yaml", ".yml", ".toml", ".txt":
		default:
			return nil
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
		lower := strings.ToLower(body)

		for hint, provider := range providerHints {
			if strings.Contains(lower, hint) {
				add(&s.Providers, provider)
			}
		}
		for hint, fw := range frameworkHints {
			if strings.Contains(lower, hint) {
				add(&s.Frameworks, fw)
			}
		}
		for _, m := range modelRe.FindAllStringSubmatch(body, -1) {
			add(&s.Models, m[1])
		}
		if name == ".mcp.json" || strings.Contains(name, "mcp") && ext == ".json" {
			rel, _ := filepath.Rel(root, p)
			add(&s.MCPFiles, rel)
		}
		// A long triple-quoted string mentioning "you are" is usually a system prompt.
		if strings.Contains(lower, "you are ") && len(body) > 200 {
			rel, _ := filepath.Rel(root, p)
			add(&s.Prompts, rel)
		}
		return nil
	})

	sort.Strings(s.Providers)
	sort.Strings(s.Models)
	sort.Strings(s.Frameworks)
	sort.Strings(s.Prompts)
	return s
}

// writeAdoptedManifest produces a manifest that is honest about what is not yet known.
func writeAdoptedManifest(root, id string, s adoptScan) error {
	dir := filepath.Join(root, "agentarch", "project", "agents", id)
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		return err
	}

	provider, model := "unknown", "unknown"
	if len(s.Providers) == 1 {
		provider = s.Providers[0]
	}
	if len(s.Models) == 1 {
		model = s.Models[0]
	}

	var found strings.Builder
	if len(s.Providers) > 0 {
		fmt.Fprintf(&found, "#   providers seen:  %s\n", strings.Join(s.Providers, ", "))
	}
	if len(s.Models) > 0 {
		shown := s.Models
		if len(shown) > 5 {
			shown = shown[:5]
		}
		fmt.Fprintf(&found, "#   models seen:     %s\n", strings.Join(shown, ", "))
	}
	if len(s.Frameworks) > 0 {
		fmt.Fprintf(&found, "#   frameworks seen: %s  (see agentarch/std/adapters/)\n",
			strings.Join(s.Frameworks, ", "))
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
