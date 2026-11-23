// Package plugin installs the same reviewed skills and MCP contract into supported hosts.
// It does not change a host's approval policy or claim to sandbox its other tools.
package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
)

const ServerVersion = "1.0.1"

// ToolDescriptions is the single source for discovery and pinned description hashes.
// Effects are classified before implementation: the first three read, the last two write.
var ToolDescriptions = map[string]string{
	"inspect_project":   "Inspect the configured project and report its local structure and governance state without changing files.",
	"get_workflow":      "Read the current workflow and its recorded tasks, decisions, and evidence without changing state.",
	"get_next_task":     "Read the next eligible workflow task and its prerequisites without starting or completing it.",
	"record_checkpoint": "Record a bounded workflow progress checkpoint; this does not certify verification or independent review.",
	"verify_project":    "Run the project's configured verification checks and record their results for the current revision.",
}

type Capabilities struct {
	Skills                bool `json:"skills"`
	MCPStdio              bool `json:"mcp_stdio"`
	GlobalToolEnforcement bool `json:"global_tool_enforcement"`
	SandboxEnforcement    bool `json:"sandbox_enforcement"`
}

type Descriptor struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	SkillsPath    string `json:"skills_path"`
	ConfigPath    string `json:"config_path"`
	ConfigFormat  string `json:"config_format"`
	// HooksPath is the host settings file that receives the real-time guardrail hooks; empty
	// for a host whose hook protocol `agentarch hook` does not speak.
	HooksPath string `json:"hooks_path,omitempty"`
	// AgentsPath is where host subagent definitions (the independent reviewer, the persona
	// user) are installed; empty for a host without file-defined subagents.
	AgentsPath    string       `json:"agents_path,omitempty"`
	Capabilities  Capabilities `json:"capabilities"`
	Documentation []string     `json:"documentation"`
	Notes         string       `json:"notes"`
}

// Options contains deployment facts supplied by the user. No owner or model is inferred.
type Options struct {
	DryRun        bool
	Owner         string
	Contact       string
	ModelProvider string
	ModelID       string
	Binary        string
	ServerVersion string
}

type Change struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type Report struct {
	Host      string   `json:"host"`
	Installed bool     `json:"installed"`
	DryRun    bool     `json:"dry_run"`
	Changes   []Change `json:"changes"`
	Issues    []string `json:"issues"`
	Backups   []string `json:"backups"`
}

func Hosts() []string { return []string{"codex", "claude", "gemini", "external"} }

func Describe(host string) (Descriptor, error) {
	var d Descriptor
	if !contains(Hosts(), host) {
		return d, fmt.Errorf("unknown plugin host %q (choose codex, claude, gemini, external)", host)
	}
	b, err := agentarch.Content.ReadFile("content/plugins/" + host + ".json")
	if err == nil {
		err = json.Unmarshal(b, &d)
	}
	return d, err
}

// Install preflights the complete change before writing. Existing unrelated JSON values
// and TOML text survive, and a changed managed entry is a conflict, never an overwrite.
func Install(root, host string, opts Options) (Report, error) {
	r := Report{Host: host, DryRun: opts.DryRun}
	d, err := Describe(host)
	if err != nil {
		return r, err
	}
	root, err = canonicalRoot(root)
	if err != nil {
		return r, err
	}
	if err = validateOptions(&opts); err != nil {
		return r, err
	}
	previous, err := readState(root, host)
	if err != nil {
		return r, err
	}
	wanted, err := payload(root, d, opts)
	if err != nil {
		return r, err
	}
	next := state{Version: 1, Host: host, Entries: map[string]entry{}}
	var changes []mutation
	for _, rel := range sortedKeys(wanted) {
		body := wanted[rel]
		old, exists, err := readSafe(root, rel)
		if err != nil {
			return r, err
		}
		prior, managed := previous.Entries[rel]
		if managed && prior.Kind == "file" && (!exists || digest(old) != prior.Hash) {
			return r, fmt.Errorf("managed file changed or missing: %s; restore it before installing", rel)
		}
		if !managed && exists && string(old) != string(body) {
			return r, fmt.Errorf("refusing to replace an existing file: %s", rel)
		}
		e := entry{Kind: "file", Hash: digest(body), Preserve: exists && !managed}
		if managed {
			e.Preserve = prior.Preserve
		}
		next.Entries[rel] = e
		if !exists || string(old) != string(body) {
			changes = append(changes, mutation{Path: rel, Data: body})
		}
	}
	// Remove only unchanged, owned files retired by a newer content release.
	for rel, e := range previous.Entries {
		if _, ok := next.Entries[rel]; ok || e.Kind != "file" || e.Preserve {
			continue
		}
		old, exists, err := readSafe(root, rel)
		if err != nil {
			return r, err
		}
		if !exists || digest(old) != e.Hash {
			return r, fmt.Errorf("retired managed file changed: %s", rel)
		}
		changes = append(changes, mutation{Path: rel, Delete: true})
	}
	old, exists, err := readSafe(root, d.ConfigPath)
	if err != nil {
		return r, err
	}
	fragment := serverConfig(root, host, opts)
	var config []byte
	var e entry
	if d.ConfigFormat == "toml" {
		config, e, err = mergeTOML(old, fragment, previous.Entries[d.ConfigPath], false)
	} else {
		config, e, err = mergeJSON(old, fragment, previous.Entries[d.ConfigPath], false)
	}
	if err != nil {
		return r, fmt.Errorf("%s: %w", d.ConfigPath, err)
	}
	e.Existed = exists
	if prior, ok := previous.Entries[d.ConfigPath]; ok {
		e.Existed = prior.Existed
	}
	next.Entries[d.ConfigPath] = e
	if string(config) != string(old) {
		changes = append(changes, mutation{Path: d.ConfigPath, Data: config})
	}
	if d.HooksPath != "" {
		oldHooks, hooksExist, err := readSafe(root, d.HooksPath)
		if err != nil {
			return r, err
		}
		body, he, err := mergeHooks(oldHooks, hookFragment(host, opts), previous.Entries[d.HooksPath], false)
		if err != nil {
			return r, fmt.Errorf("%s: %w", d.HooksPath, err)
		}
		he.Existed = hooksExist
		if prior, ok := previous.Entries[d.HooksPath]; ok {
			he.Existed = prior.Existed
		}
		next.Entries[d.HooksPath] = he
		if string(body) != string(oldHooks) {
			changes = append(changes, mutation{Path: d.HooksPath, Data: body})
		}
	}
	// Private state can contain hashes and config backups, but no captured agent content.
	stateBytes, _ := json.MarshalIndent(next, "", "  ")
	stateBytes = append(stateBytes, '\n')
	statePath := stateFile(host)
	oldState, _, err := readSafe(root, statePath)
	if err != nil {
		return r, err
	}
	if string(oldState) != string(stateBytes) {
		changes = append(changes, mutation{Path: statePath, Data: stateBytes, Private: true})
	}
	ignore := ".agentarch/plugin-state/.gitignore"
	if current, exists, err := readSafe(root, ignore); err != nil {
		return r, err
	} else if exists && string(current) != "*\n!.gitignore\n" {
		return r, errors.New("plugin-state .gitignore changed; backups must remain excluded from version control")
	} else if !exists {
		changes = append(changes, mutation{Path: ignore, Data: []byte("*\n!.gitignore\n"), Private: true})
	}
	r.Installed = true
	err = apply(root, host, changes, &r)
	return r, err
}

// Check is local and read-only. It checks installed artifacts, not host authentication,
// effective sandbox policy, or whether a running host has reloaded its configuration.
func Check(root, host string) (Report, error) {
	r := Report{Host: host}
	if _, err := Describe(host); err != nil {
		return r, err
	}
	root, err := canonicalRoot(root)
	if err != nil {
		return r, err
	}
	s, err := readState(root, host)
	if err != nil {
		return r, err
	}
	r.Installed = len(s.Entries) > 0
	if !r.Installed {
		r.Issues = append(r.Issues, "plugin is not installed")
		return r, nil
	}
	for _, rel := range sortedEntries(s.Entries) {
		e := s.Entries[rel]
		b, exists, err := readSafe(root, rel)
		if err != nil {
			return r, err
		}
		if !exists {
			r.Issues = append(r.Issues, "missing: "+rel)
			continue
		}
		hash := digest(b)
		if e.Kind == "json" {
			hash, err = jsonFragmentHash(b)
		}
		if e.Kind == "toml" {
			hash, err = tomlFragmentHash(b)
		}
		if e.Kind == "hooks" {
			hash, err = hooksFragmentHash(b)
		}
		if err != nil {
			r.Issues = append(r.Issues, rel+": "+err.Error())
			continue
		}
		if hash != e.Hash {
			r.Issues = append(r.Issues, "managed content changed: "+rel)
		}
	}
	return r, nil
}

// Remove deletes only unchanged entries owned by this install. User changes anywhere
// outside the owned config fragment remain intact, including changes made after install.
func Remove(root, host string, opts Options) (Report, error) {
	r := Report{Host: host, DryRun: opts.DryRun}
	if _, err := Describe(host); err != nil {
		return r, err
	}
	root, err := canonicalRoot(root)
	if err != nil {
		return r, err
	}
	s, err := readState(root, host)
	if err != nil {
		return r, err
	}
	var changes []mutation
	for _, rel := range sortedEntries(s.Entries) {
		e := s.Entries[rel]
		if e.Preserve {
			continue
		}
		old, exists, err := readSafe(root, rel)
		if err != nil {
			return r, err
		}
		if !exists {
			continue
		}
		switch e.Kind {
		case "file":
			if digest(old) != e.Hash {
				return r, fmt.Errorf("refusing to remove changed file: %s", rel)
			}
			changes = append(changes, mutation{Path: rel, Delete: true})
		case "json", "toml":
			var body []byte
			if e.Kind == "json" {
				body, _, err = mergeJSON(old, nil, e, true)
			} else {
				body, _, err = mergeTOML(old, nil, e, true)
			}
			if err != nil {
				return r, fmt.Errorf("%s: %w", rel, err)
			}
			changes = append(changes, mutation{Path: rel, Data: body, Delete: !e.Existed && emptyConfig(body, e.Kind)})
		case "hooks":
			body, _, err := mergeHooks(old, nil, e, true)
			if err != nil {
				return r, fmt.Errorf("%s: %w", rel, err)
			}
			changes = append(changes, mutation{Path: rel, Data: body, Delete: !e.Existed && emptyConfig(body, "json")})
		default:
			return r, fmt.Errorf("unknown managed entry kind %q", e.Kind)
		}
	}
	if len(s.Entries) > 0 {
		changes = append(changes, mutation{Path: stateFile(host), Delete: true, Private: true})
	}
	err = apply(root, host, changes, &r)
	return r, err
}

func validateOptions(o *Options) error {
	for _, field := range []struct{ name, value string }{{"owner", o.Owner}, {"contact", o.Contact}, {"model-provider", o.ModelProvider}, {"model-id", o.ModelID}} {
		if strings.TrimSpace(field.value) == "" || strings.ContainsAny(field.value, "\x00\r\n") {
			return fmt.Errorf("%s is required and must be a single line supplied by the user", field.name)
		}
	}
	if len(strings.TrimSpace(o.Owner)) < 3 || len(strings.TrimSpace(o.Contact)) < 3 {
		return errors.New("owner must name an accountable person and contact must be usable")
	}
	if strings.EqualFold(o.ModelID, "latest") || strings.Contains(o.ModelID, "*") || strings.HasSuffix(strings.ToLower(o.ModelID), "-latest") {
		return errors.New("model-id must identify an exact pinned model")
	}
	if o.Binary == "" {
		o.Binary = "agentarch"
	}
	if strings.ContainsAny(o.Binary, "\x00\r\n") {
		return errors.New("invalid binary path")
	}
	if o.ServerVersion == "" {
		o.ServerVersion = ServerVersion
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`).MatchString(o.ServerVersion) {
		return errors.New("server-version must be an exact semantic version")
	}
	return nil
}

func payload(root string, d Descriptor, o Options) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(agentarch.Content, "content/skills", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		b, err := agentarch.Content.ReadFile(path)
		if err == nil {
			files[filepath.ToSlash(filepath.Join(d.SkillsPath, strings.TrimPrefix(path, "content/skills/")))] = b
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if d.AgentsPath != "" {
		agents, _ := fs.Glob(agentarch.Content, "content/plugins/agents/*.md")
		for _, path := range agents {
			b, err := agentarch.Content.ReadFile(path)
			if err != nil {
				return nil, err
			}
			files[d.AgentsPath+"/"+strings.TrimPrefix(path, "content/plugins/agents/")] = b
		}
	}
	b, err := agentarch.Content.ReadFile("content/plugins/" + d.ID + ".json")
	if err != nil {
		return nil, err
	}
	files["agentarch/project/plugins/"+d.ID+"/descriptor.json"] = b
	contracts, err := contractFiles(root, d.ID, o)
	for path, b := range contracts {
		files[path] = b
	}
	return files, err
}

// serverRootArg is the --root written into the host configuration. Claude Code's .mcp.json
// is shared through the repository, so an absolute path would point at the installer's own
// machine for everyone who clones it. Claude Code starts stdio servers in the directory it
// was opened in (checked with 2.1.252), and `mcp serve` searches upward from there for the
// project, so "." resolves correctly from the root or any subfolder. Claude Code does not
// expand ${CLAUDE_PROJECT_DIR} in server arguments, so that form is not used. The other
// hosts keep the absolute root until their launch directory has been verified the same way.
func serverRootArg(root, host string) string {
	if host == "claude" {
		return "."
	}
	return root
}

func serverConfig(root, host string, o Options) map[string]any {
	m := map[string]any{"command": o.Binary, "args": []string{"mcp", "serve", "--root", serverRootArg(root, host)}}
	if host == "gemini" {
		m["includeTools"] = toolNames()
	}
	if host == "claude" {
		m["type"] = "stdio"
	}
	return m
}

func toolNames() []string {
	n := make([]string, 0, len(ToolDescriptions))
	for k := range ToolDescriptions {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
func sortedKeys(m map[string][]byte) []string {
	n := make([]string, 0, len(m))
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
func sortedEntries(m map[string]entry) []string {
	n := make([]string, 0, len(m))
	for k := range m {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// reviewedDate is captured once per install in the allowlist. Reinstalling an unchanged
// deployment preserves its original review date rather than silently renewing approval.
func reviewedDate(root, host string) string {
	b, exists, _ := readSafe(root, "agentarch/project/plugins/"+host+"/mcp.allowlist.yaml")
	if exists {
		var v map[string]any
		if json.Unmarshal(b, &v) == nil {
			if servers, ok := v["servers"].([]any); ok && len(servers) > 0 {
				if s, ok := servers[0].(map[string]any); ok {
					if date, ok := s["reviewed_at"].(string); ok {
						return date
					}
				}
			}
		}
	}
	return time.Now().UTC().Format("2006-01-02")
}
