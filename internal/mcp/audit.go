// Package mcp audits a project's MCP allowlist.
//
// An MCP server contributes text the model treats as authoritative — tool names, descriptions,
// parameter documentation. Connecting one is closer to importing a dependency that can write
// your prompt than to configuring a client, and the protocol attaches no version to a
// description. That gap is what this package exists to close.
package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Allowlist is the reviewed, auditable source of truth. .mcp.json is generated from it.
type Allowlist struct {
	SchemaVersion string   `yaml:"schema_version" json:"schema_version"`
	Default       string   `yaml:"default" json:"default"`
	Servers       []Server `yaml:"servers" json:"servers"`
}

type Server struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Transport   string   `yaml:"transport" json:"transport"`
	Command     string   `yaml:"command,omitempty" json:"command,omitempty"`
	Args        []string `yaml:"args,omitempty" json:"args,omitempty"`
	URL         string   `yaml:"url,omitempty" json:"url,omitempty"`

	Pin struct {
		Package   string `yaml:"package,omitempty" json:"package,omitempty"`
		Version   string `yaml:"version" json:"version"`
		Integrity string `yaml:"integrity,omitempty" json:"integrity,omitempty"`
	} `yaml:"pin" json:"pin"`

	Trust      string `yaml:"trust" json:"trust"`
	ReviewedAt string `yaml:"reviewed_at" json:"reviewed_at"`
	Reviewer   string `yaml:"reviewer" json:"reviewer"`

	ToolsAllow           []string          `yaml:"tools_allow" json:"tools_allow"`
	ToolsDeny            []string          `yaml:"tools_deny,omitempty" json:"tools_deny,omitempty"`
	ToolDescriptionSHA   map[string]string `yaml:"tool_description_sha256,omitempty" json:"tool_description_sha256,omitempty"`
	ResourcesAllow       []string          `yaml:"resources_allow,omitempty" json:"resources_allow,omitempty"`
	PromptsAllow         []string          `yaml:"prompts_allow,omitempty" json:"prompts_allow,omitempty"`
	EnvAllow             []string          `yaml:"env_allow,omitempty" json:"env_allow,omitempty"`
	NetworkEgress        []string          `yaml:"network_egress,omitempty" json:"network_egress,omitempty"`
	RequiresHumanApprove bool              `yaml:"requires_human_approval,omitempty" json:"requires_human_approval,omitempty"`
	Sandbox              string            `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
}

// Load reads the allowlist. A project with no allowlist and no MCP servers is fine; a project
// with servers and no allowlist is caught by control.ai.mcp.allowlist_enforced.
func Load(root string) (*Allowlist, string, error) {
	path := filepath.Join(root, "agentarch", "project", "mcp", "allowlist.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	var a Allowlist
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return &a, path, nil
}

// Finding is one problem with the allowlist or with a live server.
type Finding struct {
	Server   string `json:"server"`
	Tool     string `json:"tool,omitempty"`
	ID       string `json:"id"`
	Message  string `json:"message"`
	Fix      string `json:"fix,omitempty"`
	Critical bool   `json:"critical"`
}

func (f Finding) String() string {
	loc := f.Server
	if f.Tool != "" {
		loc += "/" + f.Tool
	}
	s := fmt.Sprintf("%-16s %s\n    %s", f.ID, loc, f.Message)
	if f.Fix != "" {
		s += "\n    fix: " + f.Fix
	}
	return s
}

// StaticAudit checks the allowlist on its own — no network, no processes.
func StaticAudit(a *Allowlist, now time.Time, maxReviewAgeDays int) []Finding {
	var out []Finding

	if a.Default != "deny" {
		out = append(out, Finding{
			ID: "MCP-DEFAULT", Message: fmt.Sprintf("default is %q", a.Default),
			Fix:      "set default: deny. An allowlist that defaults to allow is a list, not an allowlist.",
			Critical: true,
		})
	}

	seen := map[string]bool{}
	for _, s := range a.Servers {
		if seen[s.Name] {
			out = append(out, Finding{Server: s.Name, ID: "MCP-DUP",
				Message: "server name declared twice", Critical: true})
		}
		seen[s.Name] = true

		if s.Transport == "stdio" {
			v := strings.ToLower(s.Pin.Version)
			if v == "" || v == "latest" || v == "*" || v == "main" || v == "master" || v == "next" {
				out = append(out, Finding{Server: s.Name, ID: "MCP-PIN",
					Message: fmt.Sprintf("version is %q", s.Pin.Version),
					Fix: "pin an exact version. A floating tag means the server you reviewed " +
						"and the server you run are different programs.",
					Critical: true})
			}
			// A version pinned in the allowlist but not in the launch arguments is a pin
			// that documents an intention nothing enforces.
			if s.Pin.Version != "" && len(s.Args) > 0 {
				joined := strings.Join(s.Args, " ")
				if !strings.Contains(joined, s.Pin.Version) &&
					(strings.Contains(joined, "@latest") || !strings.Contains(joined, "@")) {
					out = append(out, Finding{Server: s.Name, ID: "MCP-PIN-ARGS",
						Message: "pin.version is recorded but the launch arguments do not carry it",
						Fix:     fmt.Sprintf("pass the pinned version in args, e.g. %s@%s", s.Pin.Package, s.Pin.Version)})
				}
			}
		}

		if len(s.ToolsAllow) == 0 {
			out = append(out, Finding{Server: s.Name, ID: "MCP-TOOLS",
				Message: "no tools_allow entries",
				Fix: "enumerate exactly the tools you accept. A tool the server starts offering " +
					"later should require a review, not arrive silently.",
				Critical: true})
		}

		for _, tool := range s.ToolsAllow {
			if _, ok := s.ToolDescriptionSHA[tool]; !ok {
				out = append(out, Finding{Server: s.Name, Tool: tool, ID: "MCP-DESC-HASH",
					Message: "no recorded description digest",
					Fix: "run `agentarch mcp audit --probe --record` to capture what the " +
						"description says today, after reading it.",
					Critical: true})
			}
		}

		if s.Reviewer == "" || s.ReviewedAt == "" {
			out = append(out, Finding{Server: s.Name, ID: "MCP-REVIEW",
				Message: "no reviewer or no review date",
				Fix:     "review is something a person did, not a state a file is in. Record both."})
		} else if d, err := time.Parse("2006-01-02", s.ReviewedAt); err == nil {
			if age := int(now.Sub(d).Hours() / 24); age > maxReviewAgeDays {
				out = append(out, Finding{Server: s.Name, ID: "MCP-STALE",
					Message: fmt.Sprintf("last reviewed %d days ago", age),
					Fix:     "re-read the current tool descriptions and update reviewed_at."})
			}
		}

		if s.Trust == "community" && (s.Sandbox == "" || s.Sandbox == "none") {
			out = append(out, Finding{Server: s.Name, ID: "MCP-SANDBOX",
				Message:  "community-trust server runs unsandboxed",
				Fix:      "run it in a container or subprocess. Nobody you can call is accountable for its contents.",
				Critical: true})
		}

		if s.EnvAllow == nil && s.Transport == "stdio" {
			out = append(out, Finding{Server: s.Name, ID: "MCP-ENV",
				Message: "env_allow is not declared, so the server may inherit the whole environment",
				Fix:     "list the variables it needs. Use an empty list if it needs none."})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Server != out[j].Server {
			return out[i].Server < out[j].Server
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ---------------------------------------------------------------- probe

// liveTool is what a server currently says about one of its tools.
type liveTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DescriptionDigest is how a description is reduced to a comparable value. Whitespace is
// normalised so a reformat does not read as tampering, but nothing else is — a changed word is
// a changed instruction to the model.
func DescriptionDigest(s string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(s), " ")))
	return hex.EncodeToString(sum[:])
}

// Probe connects to a stdio server and lists its tools over MCP.
//
// This is the only part of agentarch that starts a process, and it is opt-in for that reason:
// auditing a supply chain by running the supply chain has to be a decision, not a default.
func Probe(s Server, timeout time.Duration) ([]liveTool, error) {
	if s.Transport != "stdio" {
		return nil, fmt.Errorf("probing %s transport is not supported yet", s.Transport)
	}
	if s.Command == "" {
		return nil, fmt.Errorf("no command to run")
	}

	cmd := exec.Command(s.Command, s.Args...)

	// Pass through only what the allowlist permits. A probe that leaked the full
	// environment would be demonstrating the very failure the control exists to prevent.
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	for _, k := range s.EnvAllow {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start server: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(b, '\n'))
		return err
	}

	if err := send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "agentarch-audit", "version": "1"},
		},
	}); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(stdout)
	done := make(chan struct{})
	var tools []liveTool
	var readErr error

	go func() {
		defer close(done)
		initialised := false
		for {
			var msg map[string]any
			if err := dec.Decode(&msg); err != nil {
				readErr = err
				return
			}
			id, _ := msg["id"].(float64)
			if id == 1 && !initialised {
				initialised = true
				_ = send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
				_ = send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
				continue
			}
			if id == 2 {
				res, _ := msg["result"].(map[string]any)
				list, _ := res["tools"].([]any)
				for _, t := range list {
					tm, _ := t.(map[string]any)
					name, _ := tm["name"].(string)
					desc, _ := tm["description"].(string)
					tools = append(tools, liveTool{Name: name, Description: desc})
				}
				return
			}
		}
	}()

	select {
	case <-done:
		if len(tools) == 0 && readErr != nil {
			return nil, fmt.Errorf("no tool list received: %w", readErr)
		}
		return tools, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out after %s", timeout)
	}
}

// CompareLive is where the rug pull is caught: what the server serves now, against what a human
// approved.
func CompareLive(s Server, live []liveTool) []Finding {
	var out []Finding
	byName := map[string]liveTool{}
	for _, t := range live {
		byName[t.Name] = t
	}

	for _, tool := range s.ToolsAllow {
		lt, present := byName[tool]
		if !present {
			out = append(out, Finding{Server: s.Name, Tool: tool, ID: "MCP-GONE",
				Message: "allowlisted tool is no longer offered by the server",
				Fix:     "remove it from tools_allow, or find out why it disappeared."})
			continue
		}
		want, recorded := s.ToolDescriptionSHA[tool]
		if !recorded {
			continue // already reported by the static audit
		}
		if got := DescriptionDigest(lt.Description); got != want {
			out = append(out, Finding{Server: s.Name, Tool: tool, ID: "MCP-RUGPULL",
				Message: fmt.Sprintf(
					"description changed since review: approved %s…, now serving %s…",
					want[:12], got[:12]),
				Fix: "read the new description before accepting it. A server can serve a benign " +
					"description during review and a hostile one afterwards, and nothing else " +
					"in the protocol would notice.",
				Critical: true})
		}
	}

	// A tool the server offers that nobody allowlisted is not itself an incident, but it is
	// the shape silent capability growth takes.
	allowed := map[string]bool{}
	for _, t := range s.ToolsAllow {
		allowed[t] = true
	}
	for _, t := range live {
		if !allowed[t.Name] {
			out = append(out, Finding{Server: s.Name, Tool: t.Name, ID: "MCP-NEW",
				Message: "server offers a tool that is not allowlisted; it will be refused",
				Fix:     "review it and add it to tools_allow, or leave it denied."})
		}
	}
	return out
}

// RenderMCPJSON produces the runtime client config from the allowlist, so the auditable
// document is the source and the runtime config is the derivative. Two hand-maintained files
// agree right up until one of them is edited.
func RenderMCPJSON(a *Allowlist) ([]byte, error) {
	servers := map[string]any{}
	for _, s := range a.Servers {
		entry := map[string]any{}
		switch s.Transport {
		case "stdio":
			entry["command"] = s.Command
			if len(s.Args) > 0 {
				entry["args"] = s.Args
			}
			if len(s.EnvAllow) > 0 {
				env := map[string]string{}
				for _, k := range s.EnvAllow {
					env[k] = "${" + k + "}"
				}
				entry["env"] = env
			}
		default:
			entry["type"] = s.Transport
			entry["url"] = s.URL
		}
		servers[s.Name] = entry
	}
	doc := map[string]any{"mcpServers": servers}
	return json.MarshalIndent(doc, "", "  ")
}
