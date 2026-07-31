# Adapter: Pydantic AI

Typed by construction, which makes the structured-output and payload-schema controls nearly free. Dependencies are the identity boundary.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```python
agent = Agent(
    manifest["model"]["id"],
    system_prompt=verified_prompt(manifest),
    deps_type=Session,
    output_type=TriageReply,        # structured output enforced by the type
)
```

## 2. Tools and the permission check

```python
@agent.tool
async def search_orders(ctx: RunContext[Session], order_reference: str) -> Orders:
    # Identity from deps. A tool taking a customer id as a parameter would let the model choose it.
    return await orders_api.search(ctx.deps.customer_id, order_reference)
```

## 3. The three guardrail points

Input: validate before `agent.run`. Output: an output validator, or the `output_type` itself
for format conformance. Action: a wrapper around the tool body that consults the `.tool.yaml`
before doing anything.

`output_type` covers `genai.output_schema_enforced` at the type level, which is the cheapest
guardrail in this document.

## 4. Telemetry

Pydantic AI instruments with OTel natively. Pin the semconv version and disable content capture
explicitly — the default in an observability integration is rarely the default you want in a
privacy assessment.

## 5. Handoff and approval

Programmatic hand-off: call the next agent with a typed payload. The type is the contract, so
`payload_schema` is satisfied by construction — which is the strongest form this control takes
anywhere.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
