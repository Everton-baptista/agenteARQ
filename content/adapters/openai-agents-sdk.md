# Adapter: OpenAI Agents SDK

Agents, handoffs and guardrails are first-class here, which makes most of the mapping direct. The one thing to watch is that the SDK's guardrails run around the model, so the action point still needs building.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```python
agent = Agent(
    name=manifest["id"],
    instructions=verified_prompt(manifest),   # hash-checked; raises on mismatch
    model=manifest["model"]["id"],            # pinned, never an alias
)
```

`instructions` is the system prompt. Retrieved content belongs in the user turn, inside a
delimited block — never interpolated into `instructions`.

## 2. Tools and the permission check

```python
@function_tool
def search_orders(order_reference: str, ctx: RunContextWrapper[Session]) -> dict:
    # Identity from the run context, not from a model-supplied argument.
    return orders_api.search(customer_id=ctx.context.customer_id, ref=order_reference)
```

Generate the docstring and signature from the `.tool.yaml` so the model-facing description and
the reviewed artifact cannot diverge.

## 3. The three guardrail points

`input_guardrail` and `output_guardrail` decorators cover two of the three points.

The **action** point is not covered by the SDK and has to be built: wrap the tool function, or
use a tool-call hook, so that permissions, domain limits and approval are checked before the
implementation runs. This is the point that matters most, and the one the framework leaves to
you.

## 4. Telemetry

The SDK's tracing exports to its own backend by default. Add an OTel processor so the same
spans reach your collector with a pinned semconv version, and set `capture_content` off unless
someone decided otherwise.

## 5. Handoff and approval

`handoffs=[...]` maps onto `handoff.hands_off_to`. Give each one an input type — the SDK
supports typed handoff inputs, which is exactly `payload_schema`. Authority does not transfer
automatically: the receiving agent gets its own tool list, and that list is what
`authority: read_only` has to mean in practice.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
