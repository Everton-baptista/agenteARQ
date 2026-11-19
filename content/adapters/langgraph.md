# Adapter: LangGraph

A graph of nodes over shared state. The state object is where agentarch attaches: it is the one thing every node sees, so it is where the manifest, the session identity and the guardrail verdicts belong.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

State carries the verified prompt, so no node can substitute its own.

```python
class AgentState(TypedDict):
    manifest: dict
    system_prompt: str          # verified against sha256 at graph construction
    session: Session            # identity lives here, never in model output
    messages: Annotated[list, add_messages]

def build_graph(manifest_path: str):
    manifest, prompt = load_verified(manifest_path)   # raises on hash mismatch
    ...
```

Retrieved documents go into a separate state key and are rendered into a delimited block by the
node that builds the message — never appended to `system_prompt`.

## 2. Tools and the permission check

Generate the tool from the `.tool.yaml` and put the permission check inside the node, before
the call, not in the tool function itself:

```python
def tool_node(state: AgentState):
    for call in state["messages"][-1].tool_calls:
        spec = TOOLS[call["name"]]

        verdict = action_guardrail(call, spec, state["session"])   # fail_closed
        if not verdict.ok:
            return {"messages": [ToolMessage(content=verdict.reason, tool_call_id=call["id"])]}

        if spec["effect"] in ("irreversible", "money", "communication"):
            return interrupt({"approval_required": spec["id"], "args": call["args"]})

        args = {**call["args"], "customer_id": state["session"].customer_id}
        ...
```

## 3. The three guardrail points

Input guardrail: a node before the agent node, with a conditional edge to an escalate node.
Output guardrail: a node after. Action guardrail: inside the tool node, as above.

Do not put a guardrail in a graph node that also calls the model — the check and the thing it
checks would share a compromise.

## 4. Telemetry

`langgraph` emits callbacks; bridge them to OTel spans rather than relying on a vendor
integration, so `semconv_version` stays pinned by you.

## 5. Handoff and approval

LangGraph's `interrupt` is the human-approval primitive — it maps directly onto
`approval.required_when`. Set a deadline alongside it: an interrupt with no timeout is a task
that waits forever, and `on_timeout: deny` is what the tool spec asked for.

Handoff is a conditional edge to another compiled graph. Validate the payload against
`payload_schema` at the boundary; a graph that trusts an untyped hand-off has no contract.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
