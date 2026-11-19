# Adapter: Google Agent Development Kit

Callbacks are the extension point, and they line up with the three guardrail points cleanly.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```python
agent = LlmAgent(
    name=manifest["id"],
    model=manifest["model"]["id"],
    instruction=verified_prompt(manifest),
)
```

## 2. Tools and the permission check

`FunctionTool` and `MCPToolset`. Keep the `.tool.yaml` as the source for the description and the
schema, and read `effect` in the before-tool callback.

## 3. The three guardrail points

```python
agent = LlmAgent(
    ...,
    before_model_callback=input_guardrail,     # fail_closed
    after_model_callback=output_guardrail,     # per the manifest's fail_mode
    before_tool_callback=action_guardrail,     # fail_closed; the last checkpoint
)
```

`before_tool_callback` returning a dict short-circuits the call — that is the deny path.

## 4. Telemetry

ADK emits its own telemetry; add an OTel exporter and pin the semconv version yourself rather
than inheriting whatever the runtime currently emits.

## 5. Handoff and approval

`sub_agents` and `transfer_to_agent` are the handoff mechanism. The transfer is decided by the
model, so the payload contract has to be enforced in the callback rather than assumed —
a transfer the model chose is not a transfer that was declared.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
