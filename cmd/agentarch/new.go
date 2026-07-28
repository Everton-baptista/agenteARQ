package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// slugRe is the id shape the schemas accept. Rejecting a bad id here, with an explanation, is
// better than scaffolding a directory that fails validation for a reason the message does not
// mention.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var toolIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func cmdNew(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: agentarch new agent|tool|adr <name>")
		return exitUsage
	}
	switch args[0] {
	case "agent":
		return cmdNewAgent(args[1:])
	case "tool":
		return cmdNewTool(args[1:])
	case "adr":
		return cmdNewADR(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown: agentarch new %s (agent, tool or adr)\n", args[0])
		return exitUsage
	}
}

// template reads a scaffolding template from the project's installed std, falling back to the
// payload this binary carries.
func template(root, name string) (string, error) {
	local := filepath.Join(root, "agentarch", "std", "templates", name)
	if b, err := os.ReadFile(local); err == nil {
		return string(b), nil
	}
	src, err := contentFS(root)
	if err != nil {
		return "", err
	}
	b, err := fs.ReadFile(src, "templates/"+name)
	if err != nil {
		return "", fmt.Errorf("template %s not found: %w", name, err)
	}
	return string(b), nil
}

func fill(tpl string, vars map[string]string) string {
	for k, v := range vars {
		tpl = strings.ReplaceAll(tpl, "{{"+k+"}}", v)
	}
	return tpl
}

// projectJurisdictions reads the default recorded at init time, so `new agent` does not ask
// again for something the project already decided.
func projectJurisdictions(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return "[]"
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if v, found := strings.CutPrefix(strings.TrimSpace(line), "jurisdictions:"); found {
			return strings.TrimSpace(v)
		}
	}
	return "[]"
}

func cmdNewAgent(args []string) int {
	fs_ := flag.NewFlagSet("new agent", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	jurisdictions := fs_.String("jurisdictions", "", "comma-separated, e.g. EU,BR (defaults to the project's)")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch new agent <id>")
		return exitUsage
	}
	id := fs_.Arg(0)

	if !slugRe.MatchString(id) {
		fmt.Fprintf(os.Stderr, "%q is not a valid agent id.\n"+
			"Use lowercase kebab-case, e.g. customer-support-triage. Ids stay in English in every\n"+
			"language, so error messages and searches stay interoperable across teams.\n", id)
		return exitUsage
	}

	dir := filepath.Join(*root, "agentarch", "project", "agents", id)
	if _, err := os.Stat(dir); err == nil {
		fmt.Fprintf(os.Stderr, "agent %q already exists at %s\n", id, dir)
		return exitUsage
	}

	promptTpl, err := template(*root, "prompt.system.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}
	agentTpl, err := template(*root, "agent.spec.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}

	juris := *jurisdictions
	switch {
	case juris == "":
		juris = projectJurisdictions(*root)
	default:
		var parts []string
		for _, j := range strings.Split(juris, ",") {
			parts = append(parts, `"`+strings.ToUpper(strings.TrimSpace(j))+`"`)
		}
		juris = "[" + strings.Join(parts, ", ") + "]"
	}

	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}
	if err := os.MkdirAll(filepath.Join(dir, "evals"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}

	promptPath := filepath.Join(dir, "prompts", "system.v1.md")
	if err := os.WriteFile(promptPath, []byte(promptTpl), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}

	// Hash the prompt as written, so the manifest is internally consistent from the first
	// commit. Scaffolding something that fails AA-REF-004 immediately would teach the reader
	// that the hash is noise to be silenced rather than a check that means something.
	sum := sha256.Sum256([]byte(promptTpl))
	manifest := fill(agentTpl, map[string]string{
		"ID":            id,
		"JURISDICTIONS": juris,
		"PROMPT_SHA256": hex.EncodeToString(sum[:]),
		"TODAY":         time.Now().UTC().Format("2006-01-02"),
	})
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(manifest), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "new agent:", err)
		return exitUsage
	}

	rel, _ := filepath.Rel(*root, dir)
	fmt.Printf("created %s\n", rel)
	fmt.Printf("  agent.yaml              the contract\n")
	fmt.Printf("  prompts/system.v1.md    hashed into the manifest already\n\n")
	fmt.Printf("Fill in the fields marked TODO, then run `agentarch validate`.\n")
	fmt.Printf("It will fail until they are filled in — that is deliberate. A manifest full of\n")
	fmt.Printf("plausible defaults is worse than one that refuses to validate, because it looks\n")
	fmt.Printf("finished.\n\n")
	fmt.Printf("The two worth thinking about before the rest:\n")
	fmt.Printf("  out_of_scope     what it must refuse. A model asked to do something adjacent\n")
	fmt.Printf("                   to its purpose will usually try.\n")
	fmt.Printf("  autonomy.level   a property of the deployment, not of the model.\n")
	return exitOK
}

func cmdNewTool(args []string) int {
	fs_ := flag.NewFlagSet("new tool", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	effect := fs_.String("effect", "read", "read, write, irreversible, money or communication")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: agentarch new tool <id> [--effect read]")
		return exitUsage
	}
	id := fs_.Arg(0)

	if !toolIDRe.MatchString(id) {
		fmt.Fprintf(os.Stderr, "%q is not a valid tool id. Use lowercase snake_case, e.g. search_orders.\n", id)
		return exitUsage
	}
	valid := map[string]bool{"read": true, "write": true, "irreversible": true, "money": true, "communication": true}
	if !valid[*effect] {
		fmt.Fprintf(os.Stderr, "%q is not a valid effect.\n"+
			"  read           returns data, changes nothing\n"+
			"  write          changes state, can be undone\n"+
			"  irreversible   cannot be undone\n"+
			"  money          moves value\n"+
			"  communication  reaches a third party\n", *effect)
		return exitUsage
	}

	path := filepath.Join(*root, "agentarch", "project", "tools", id+".tool.yaml")
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "tool %q already exists at %s\n", id, path)
		return exitUsage
	}

	tpl, err := template(*root, "tool.spec.yaml")
	if err != nil {
		fmt.Fprintln(os.Stderr, "new tool:", err)
		return exitUsage
	}
	body := fill(tpl, map[string]string{"ID": id})
	body = strings.Replace(body, "\n  effect: read\n", "\n  effect: "+*effect+"\n", 1)

	// An irreversible tool needs an approval block to validate at all, so scaffold it rather
	// than leaving the reader to discover the requirement from a schema error.
	if *effect == "irreversible" || *effect == "money" || *effect == "communication" {
		body = strings.Replace(body,
			`  # Required for effect irreversible, money or communication.
  # approval:
  #   required_when: "always"   # or an expression over the input
  #   approver_role: TODO
  #   timeout_s: 900
  #   on_timeout: deny          # never allow — an unanswered request is not consent`,
			`  # Required, because this effect cannot be undone.
  approval:
    required_when: "always"     # or an expression over the input
    approver_role: TODO
    timeout_s: 900
    on_timeout: deny            # never allow — an unanswered request is not consent`, 1)
		body = strings.Replace(body, "    sandbox: subprocess", "    sandbox: container", 1)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "new tool:", err)
		return exitUsage
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "new tool:", err)
		return exitUsage
	}

	rel, _ := filepath.Rel(*root, path)
	fmt.Printf("created %s  (effect: %s)\n\n", rel, *effect)
	if *effect != "read" {
		fmt.Printf("This tool changes something. Before implementing it:\n")
		fmt.Printf("  permissions.network.egress   enumerate exact hosts; a wildcard means anything\n")
		fmt.Printf("  permissions.data_access      the narrowest set that works\n")
		fmt.Printf("  limits.domain_limits         business bounds keep a successful injection cheap\n\n")
	}
	fmt.Printf("Reference it from an agent's `tools:` list, then run `agentarch validate`.\n")
	return exitOK
}

func cmdNewADR(args []string) int {
	fs_ := flag.NewFlagSet("new adr", flag.ContinueOnError)
	root := fs_.String("root", ".", "project root")
	if err := fs_.Parse(hoistFlags(args)); err != nil {
		return exitUsage
	}
	if fs_.NArg() < 1 {
		fmt.Fprintln(os.Stderr, `usage: agentarch new adr "<title>"`)
		return exitUsage
	}
	title := strings.Join(fs_.Args(), " ")

	dir := filepath.Join(*root, "agentarch", "project", "adr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "new adr:", err)
		return exitUsage
	}
	entries, _ := os.ReadDir(dir)
	num := fmt.Sprintf("%04d", len(entries)+1)

	slug := strings.ToLower(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(title, "-"))
	slug = strings.Trim(slug, "-")

	tpl, err := template(*root, "adr.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "new adr:", err)
		return exitUsage
	}
	body := fill(tpl, map[string]string{
		"NUM": num, "TITLE": title, "TODAY": time.Now().UTC().Format("2006-01-02"),
	})

	path := filepath.Join(dir, num+"-"+slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "new adr:", err)
		return exitUsage
	}
	rel, _ := filepath.Rel(*root, path)
	fmt.Printf("created %s\n", rel)
	return exitOK
}
