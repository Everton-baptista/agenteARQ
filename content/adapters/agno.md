# Adapter: Agno

Agents and teams with a toolkit model. Toolkits bundle many tools, so effect classification and per-tool permission need attention at the point where a toolkit is registered.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```python
agent = Agent(
    model=Claude(id=manifest["model"]["id"]),
    system_message=verified_prompt(manifest),
)
```

## 2. Tools and the permission check

A toolkit registers several tools at once, which makes it easy to grant more than was reviewed.
Enumerate the functions you accept rather than taking the toolkit whole:

```python
agent = Agent(tools=[OrdersToolkit(include_tools=["search_orders"])])
```

Every accepted function needs its own `.tool.yaml`; a toolkit is not a unit of authority.

## 3. The three guardrail points

Agno has hooks around tool calls; use them for the action point. Input and output checks wrap
`agent.run`. If a hook is unavailable for a given toolkit, wrap the functions directly —
the action point is not optional.

## 4. Telemetry

Agno integrates with observability backends; add an OTel exporter and pin the semconv version so
the telemetry contract is yours rather than the vendor's.

## 5. Handoff and approval

`Team` with an orchestrator maps onto `handoff`. Set per-member tool lists so authority is
narrowed at the member, and bound the team's total steps from `autonomy.max_steps` — a team
without a combined budget can spend far more than any member's limit suggests.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
