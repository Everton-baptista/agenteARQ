# Adapter: Claude Agent SDK

Tools, permission callbacks and MCP are native, so the mapping is close. The permission callback is the action guardrail, and it is the piece worth getting exactly right.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```python
options = ClaudeAgentOptions(
    system_prompt=verified_prompt(manifest),
    model=manifest["model"]["id"],
    max_turns=manifest["autonomy"]["max_steps"],
)
```

`max_turns` is `autonomy.max_steps`. Set it from the manifest rather than a constant, so the
declared bound and the enforced bound cannot drift.

## 2. Tools and the permission check

Tools come from `@tool` definitions or from MCP servers. Either way the `.tool.yaml` stays the
contract: generate the description from it, and keep `effect` where the permission callback can
read it.

## 3. The three guardrail points

The permission callback **is** the action guardrail:

```python
async def can_use_tool(tool_name: str, input: dict, context) -> PermissionResult:
    spec = TOOLS.get(tool_name)
    if spec is None:
        return PermissionResultDeny(message="tool is not declared for this agent")

    if not egress_allowed(spec, input):
        return PermissionResultDeny(message="target is not in the egress allowlist")

    if spec["effect"] in ("irreversible", "money", "communication"):
        if not await approval.granted(spec, input, context):
            return PermissionResultDeny(message="human approval required")

    return PermissionResultAllow(updated_input={**input, "customer_id": context.customer_id})
```

Note `updated_input`: identity is injected here, server-side, rather than trusted from the
model. Input and output guardrails wrap the query and its result.

## 4. Telemetry

Wrap the query in a span and record model, tokens, cost and each permission decision.
A denied tool call is the most interesting event the system produces and the easiest to lose.

## 5. Handoff and approval

Subagents map onto `handoff`. Give each one only the tools its declared authority allows —
authority that transfers by default is not authority that was declared.

MCP servers configured here must match `agentarch/project/mcp/allowlist.yaml`; run
`agentarch sync --targets mcp_json` so `.mcp.json` is derived from the reviewed document rather
than maintained beside it.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
