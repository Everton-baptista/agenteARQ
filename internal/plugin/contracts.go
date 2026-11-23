package plugin

import (
	"encoding/json"
	"fmt"

	agentarch "github.com/Everton-baptista/agenteARQ"
)

// contractFiles renders deployment-specific contracts without interpolating data into
// instructions. JSON is a YAML subset and safely quotes names and model identifiers.
func contractFiles(root, host string, o Options) (map[string][]byte, error) {
	files := map[string][]byte{}
	base := "agentarch/project/agents/agentarch-" + host + "/"
	prompt, err := agentarch.Content.ReadFile("content/plugins/prompts/system.v1.md")
	if err != nil {
		return nil, err
	}
	files[base+"prompts/system.v1.md"] = prompt
	refs := []map[string]any{}
	hashes := map[string]string{}
	for _, name := range toolNames() {
		id := "agentarch_" + host + "_" + name
		refs = append(refs, map[string]any{"ref": "../../tools/" + id + ".tool.yaml", "approval": "policy"})
		hashes[name] = digest([]byte(ToolDescriptions[name]))
		effect := "read"
		write := []string{}
		sandbox := "subprocess"
		timeout := 4000
		if name == "record_checkpoint" || name == "verify_project" {
			effect = "write"
			write = []string{"git-dir/agentarch-workflow/**", "agentarch/project/evidence/latest.json"}
		}
		if name == "verify_project" {
			sandbox = "container"
			timeout = 1830000
		}
		tool := map[string]any{
			"id": id, "description_for_model": ToolDescriptions[name],
			"description_for_humans": "AgenteARQ MCP operation. The server bounds its own operation; host shell tools remain governed by the host.",
			"input_schema":           ToolSchemas[name], "effect": effect,
			"idempotency": map[string]any{"idempotent": effect == "read"},
			"permissions": map[string]any{"scopes": []string{"project:" + effect}, "network": map[string]any{"deny_by_default": true, "egress": []string{}}, "filesystem": map[string]any{"read": []string{"."}, "write": write}, "secrets": []string{}, "sandbox": sandbox},
			"limits":      map[string]any{"timeout_ms": timeout, "max_retries": 0, "rate_limit_rpm": 30},
			"failure":     map[string]any{"on_timeout": "fail_closed", "on_error": "fail_closed"}, "owner": o.Owner,
		}
		if err = putJSON(files, "agentarch/project/tools/"+id+".tool.yaml", map[string]any{"schema_version": "1.0", "tool": tool}); err != nil {
			return nil, err
		}
	}
	agent := map[string]any{
		"id": "agentarch-" + host, "name": "AgenteARQ project workflow in " + host,
		"owner": map[string]any{"accountable": o.Owner, "contact": o.Contact}, "stage": "internal", "system_type": "agentic_workflow",
		"purpose":      "Help the project owner produce an authorized, verified software change and a reviewable pull request.",
		"out_of_scope": []string{"Merging or deploying without separate user authorization.", "Changing host approvals or claiming global sandbox enforcement.", "Certifying the author's own work as independent review."},
		"autonomy":     map[string]any{"level": "L2_act_reversible", "max_steps": 24, "max_tool_calls": 80, "stop_conditions": []string{"Authorized deliverable has revision-bound verification and required review evidence.", "A required check fails or an essential owner decision is unavailable.", "Step, tool-call, time, or token budget is exhausted."}, "budget": map[string]any{"usd_per_run": 5.0, "tokens_per_run": 80000, "latency_p95_ms": 120000}},
		"model":        map[string]any{"provider": o.ModelProvider, "id": o.ModelID, "pinned": true},
		"prompts":      map[string]any{"system": map[string]any{"path": "prompts/system.v1.md", "version": "1.2.0", "sha256": digest(prompt)}, "variables": []map[string]any{{"name": "task_context", "source": "user", "trusted": false}}},
		"tools":        refs, "mcp": map[string]any{"allowlist_ref": "../../plugins/" + host + "/mcp.allowlist.yaml", "servers_used": []string{"agentarch"}},
		// The plugin cannot honestly assert enforcement over arbitrary host input/output.
		// Empty arrays record that boundary; local MCP action validation is declared below.
		"guardrails":    map[string]any{"input": []any{}, "output": []any{}, "action": actionGuardrails(host)},
		"privacy":       map[string]any{"processes_personal_data": false, "capture_content": false},
		"observability": map[string]any{"otel": map[string]any{"enabled": false, "capture_content": false}},
		"lifecycle":     map[string]any{"revalidate_on": []string{"model_changed", "system_prompt_changed", "provider_changed", "tool_added", "autonomy_raised", "mcp_server_added"}, "review_interval_days": 30},
	}
	if err = putJSON(files, base+"agent.yaml", map[string]any{"schema_version": "1.0", "agent": agent}); err != nil {
		return nil, err
	}
	server := map[string]any{"name": "agentarch", "transport": "stdio", "command": o.Binary, "args": []string{"mcp", "serve", "--root", serverRootArg(root, host)}, "pin": map[string]any{"package": "agentarch-mcp-contract", "version": o.ServerVersion}, "trust": "first_party", "reviewed_at": reviewedDate(root, host), "reviewer": o.Owner, "tools_allow": toolNames(), "tool_description_sha256": hashes, "resources_allow": []string{}, "prompts_allow": []string{}, "env_allow": []string{}, "network_egress": []string{}, "sandbox": "subprocess"}
	if err = putJSON(files, "agentarch/project/plugins/"+host+"/mcp.allowlist.yaml", map[string]any{"schema_version": "1.0", "default": "deny", "servers": []any{server}}); err != nil {
		return nil, err
	}
	return files, nil
}

func putJSON(files map[string][]byte, path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("render %s: %w", path, err)
	}
	files[path] = append(b, '\n')
	return nil
}

// ToolSchemas describes the minimal public MCP surface. The MCP server uses these same
// schemas; provider-specific execution APIs are intentionally not part of this contract.
var ToolSchemas = map[string]map[string]any{
	"inspect_project": {"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	"get_workflow":    {"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	"get_next_task":   {"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	"record_checkpoint": {"type": "object", "properties": map[string]any{
		"phase":             map[string]any{"type": "string", "enum": []any{"draft", "specified", "planned", "implementing", "verifying", "reviewing", "pr_ready", "blocked", "cancelled"}},
		"expected_revision": map[string]any{"type": "integer", "minimum": 0},
		"task_id":           map[string]any{"type": "string"}, "note": map[string]any{"type": "string", "maxLength": 2000},
	}, "required": []any{"phase", "expected_revision"}, "additionalProperties": false},
	"verify_project": {"type": "object", "properties": map[string]any{
		"expected_revision": map[string]any{"type": "integer", "minimum": 0},
		"image":             map[string]any{"type": "string", "pattern": `.+@sha256:[a-f0-9]{64}$`},
	}, "required": []any{"expected_revision"}, "additionalProperties": false},
}

// actionGuardrails declares what the install actually enforces. Every host gets the MCP
// operations' own validation; a host whose hooks `agentarch hook` drives also gets the pre-tool
// decisions, stated with their real, narrow scope.
func actionGuardrails(host string) []map[string]any {
	g := []map[string]any{{"control": "control.ai.tool.least_privilege", "fail_mode": "fail_closed", "note": "Applies only to AgenteARQ MCP operations. The host owns all other tool, input, and output controls."}}
	if len(hookEventsByHost[host]) > 0 {
		g = append(g, map[string]any{"control": "control.ai.tool.irreversible_requires_approval", "fail_mode": "fail_closed", "note": "Host PreToolUse hook: asks the owner (Claude Code) or blocks (Codex, which cannot ask from a hook) before push, merge, publish and deploy commands, denies edits to generated files and workflow evidence, and asks before writing secret-shaped content. It does not parse obfuscated commands; CI's agentarch check remains the guarantee."})
	}
	return g
}
