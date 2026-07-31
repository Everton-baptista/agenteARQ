# Adapter: no framework, direct provider SDK

The baseline. Everything else is a variation on this, and if a framework makes any of it harder
rather than easier, that is worth knowing before adopting it.

## 1. The versioned system prompt

Read it from the path in the manifest and verify the hash at startup. Failing closed on a
mismatch is the point: a prompt edited without a version bump is an invisible behaviour change,
and starting anyway means shipping something nobody reviewed.

```python
import hashlib, pathlib, yaml

manifest = yaml.safe_load(open("agentarch/project/agents/triage/agent.yaml"))["agent"]
spec = manifest["prompts"]["system"]
raw = pathlib.Path("agentarch/project/agents/triage", spec["path"]).read_bytes()

if hashlib.sha256(raw).hexdigest() != spec["sha256"]:
    raise SystemExit(
        f"system prompt has changed but {spec['version']} was not bumped. "
        "Run `agentarch validate` — this is AA-REF-004."
    )
SYSTEM_PROMPT = raw.decode()
```

Untrusted content never joins this string. It goes in its own delimited block:

```python
messages = [{
    "role": "user",
    "content": (
        "<retrieved_content>\n" + passages + "\n</retrieved_content>\n"
        "<customer_message>\n" + question + "\n</customer_message>"
    ),
}]
```

## 2. Tools, and where permission is checked

The `.tool.yaml` is the contract. Generate the provider's schema from it rather than writing it
twice — two hand-maintained copies agree until one is edited.

```python
spec = yaml.safe_load(open("agentarch/project/tools/search_orders.tool.yaml"))["tool"]

tool_schema = {
    "name": spec["id"],
    "description": spec["description_for_model"],
    "input_schema": spec["input_schema"],
}
```

The permission check runs **before** dispatch, and consults the spec, not the model:

```python
def dispatch(name, args, session):
    spec = TOOLS[name]

    # Identity comes from the session. A tool that accepts a customer id as an argument has
    # handed identity selection to whoever can write into the model's context.
    args = {**args, "customer_id": session.customer_id}

    if spec["effect"] in ("irreversible", "money", "communication"):
        if not approval.granted(spec, args, session):
            return {"error": "awaiting human approval", "retryable": False}

    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        if args.get(limit.replace("max_", "")) and args[...] > cap:
            return {"error": f"{limit} exceeded", "retryable": False}

    return IMPLS[name](**args)
```

## 3. The three guardrail points

```python
def run(question, session):
    if not guardrails.input_ok(question):          # fail_closed
        return escalate("input rejected")

    for step in range(manifest["autonomy"]["max_steps"]):
        response = client.messages.create(...)

        for call in response.tool_calls:
            if not guardrails.action_ok(call, TOOLS[call.name], session):   # fail_closed
                return escalate(f"tool call refused: {call.name}")
            ...

        if response.text:
            ok, text = guardrails.output_ok(response.text)                  # fail_closed
            return text if ok else escalate("output rejected")

    return escalate("step budget exhausted")
```

`guardrails.action_ok` must not call the model. A check that shares the model's context shares
its compromise.

## 4. Telemetry

```python
from opentelemetry import trace
tracer = trace.get_tracer("agentarch", "1.29.0")   # pin the semconv version

with tracer.start_as_current_span("gen_ai.invoke_agent") as span:
    span.set_attribute("gen_ai.system", manifest["model"]["provider"])
    span.set_attribute("gen_ai.request.model", manifest["model"]["id"])
    span.set_attribute("agentarch.agent.id", manifest["id"])
    span.set_attribute("gen_ai.usage.input_tokens", usage.input_tokens)
    span.set_attribute("gen_ai.usage.output_tokens", usage.output_tokens)
    # No prompt or response body: capture_content is false unless someone decided otherwise,
    # for a bounded period, with a stated reason.
```

## 5. Handoff and approval

```python
def hand_off(to_agent, payload, session):
    contract = next(h for h in manifest["handoff"]["hands_off_to"] if h["agent_id"] == to_agent)
    validate(payload, contract["payload_schema"])       # typed, or it is not a contract

    return run_agent(
        to_agent, payload,
        authority=contract["authority"],                 # read_only or delegated, never inherited
        deadline=time.time() + contract["timeout_s"],
        return_to=contract["return_point"],
    )
```

Approval shows the approver what will happen — the tool, the arguments, the amount, the
recipient — never just "approve?". On timeout it denies: an unanswered request is not consent.
