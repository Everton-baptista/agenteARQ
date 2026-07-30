# Adapter: FastAPI

FastAPI is a web framework, not an agent framework, so this file is about where the loop is
*reached from*, not how the loop is structured. Every rule exists framework-neutrally in
`standards/16-service-and-edge.md`; this is what it looks like in code, copied from
`content/blueprints/rag-support/`, which ships the service with tests.

## 0. The layout, and the one rule that makes it a layout

```
app/
  api/       transport. May import agent, domain, infra.
  agent/     the loop, prompts, tools, guardrails. May import domain, infra. Never api.
  domain/    business logic. No LLM, no HTTP.
  infra/     provider client, secrets, storage, telemetry, resilience.
contracts/openapi.json     generated
.env.example               committed: names, never values
.env                       gitignored
```

**`agent/` must not import `api/`.** Declare the layers in `agentarch.yaml` — `layout.preset:
service`, `layout.paths.edge: ["app/api/**"]`, `core: ["app/agent/**"]`, `domain`, `infra`,
`contract: "contracts/openapi.json"` — and `control.ai.api.core_transport_separated` checks the
direction. `app/cli.py` in the blueprint exists to prove the rule holds: same agent, no server.

## 1. Where the caller becomes an identity

The tenant comes out of a verified credential. Nothing in the request body influences it.

```python
# app/api/deps.py
async def current_principal(authorization: str = Header(default="")) -> Principal:
    scheme, _, token = authorization.partition(" ")
    if scheme.lower() != "bearer" or not token:
        raise HTTPException(401, "a bearer token is required",
                            headers={"WWW-Authenticate": "Bearer"})
    principal = verify(token)          # your identity provider
    if principal is None:
        raise HTTPException(401, "invalid token")   # never "expired" vs "unknown"
    return principal
```

`Principal` is defined in `app/agent/`, holds no reference to the request, and the memory
`scope_key` is derived from it — so a CLI, a worker or a test can construct the same type. A
scope key the caller can set is not a scope; it is a parameter.
→ `control.ai.api.caller_identified`

## 2. The contract, and what must not be in it

```python
class AskRequest(BaseModel):
    question: str = Field(min_length=1, max_length=4000)
    conversation_id: str | None = Field(default=None, max_length=64)
```

Two absences are the security property: **no `tenant_id`, `customer_id` or `subject`** — a field
that selects the customer cannot be overridden if it was never accepted — and **no `model`,
`temperature`, `system` or `max_steps`**, which are pinned in the manifest.
`contracts/openapi.json` is generated from the manifest's `interface` block by `agentarch sync`,
the way `.mcp.json` is generated from the allowlist. → `control.ai.api.contract_generated`

## 3. The three guardrails, and the tool permission check

They live in `app/agent/guardrails.py`, called from the loop — not from a route, and not from
FastAPI middleware, which is lost when the framework changes and never sees the tool call. The
route passes `question` and nothing else, and the loop puts it inside a delimited block:
`f"Answer this: {body.question}"` in a system prompt is one line, looks harmless, and is the
whole of invariant 2 broken.

The permission check is `action_guardrail`, called from the dispatcher once per tool call —
never in `Depends()`, which runs once per request while the model then makes four tool calls,
three of them unchecked. It never calls the model: a check that shares the model's context
shares its compromise. The transport contributes only the *authority* the check reads —
`Principal(tenant_id, subject, role)`, resolved from the credential, never from the body.

The role check runs **before** an approval is raised, or anyone can fill the approver's queue
with requests they were never allowed to make.
→ `control.ai.tool.least_privilege`, `control.ai.tool.irreversible_requires_approval`

## 4. Handoff, and approval when the approver is not in the request

In-process, handoff is a function call across a typed boundary and the transport never sees it;
the caller talks to the orchestrator, so there is deliberately no `agent_id` or `authority`
field in the request. Across services the payload is untrusted again at the receiving edge, the
budget travels as headers, authority is a *narrower* credential the callee verifies, and
`return_point`/`timeout_s` in the contract are what make a hang decidable.

An `input()` call blocks a worker until the request times out, so the run pauses and returns,
and a second request carries the decision:

```python
if outcome.status == "awaiting_approval":
    approval_id = approvals.queue.put(Pending(...))
    return AskResponse(status="awaiting_approval", approval_id=approval_id,
                       approval=outcome.approval)
```

Four properties, each a rule in `07-hitl.md`: a TTL on the record, a tenant check with a
namespaced key, single use, and an audit line of who approved what they were shown. Store it
behind a four-method interface (`app/infra/store.py`): in-memory is correct for one replica and
wrong for two.

## 5. Telemetry, and the log that is usually the leak

Log the request id, method, status, duration, tenant, and the **route template**
(`/v1/approvals/{id}`, not the id — an identifier in a path is personal data in a great many
services). Not logged: the body, the query values, the answer, the token, and the exception
message, which routinely contains the value that caused it. Disable the framework's own access
log (`logging.getLogger("uvicorn.access").disabled = True`). This is where
`capture_content: false` either holds or is a lie. → `control.ai.api.request_logging_redacted`

Spans and metrics live in `app/infra/telemetry.py`, semconv pinned, content never an attribute.
The cost figure that feeds the dashboard is the same one that enforces the budget, so the bill
and the limit cannot disagree.

## 6. The budget the manifest does not bound

`max_steps` and `usd_per_run` bound one run; a caller who issues ten thousand runs is inside
every one of them, and the first symptom is an invoice. Enforce a per-caller bound where the
identity was resolved:

```python
def enforce_caller_budget(principal: Principal = Depends(current_principal)) -> Principal:
    if len(recent) >= DEFAULT_RUNS_PER_DAY:
        raise HTTPException(429, "daily run budget exhausted",
                            headers={"Retry-After": str(WINDOW_SECONDS)})
```

→ `control.ai.api.budget_per_caller`

## 7. Health, and why it is readiness

`/health` verifies the credential resolves, the system prompt still hashes to the manifest, and
the provider circuit is not open. Readiness and liveness are different checks: an unready
replica must stop taking traffic but must not be killed — a restart loses its pending approvals.

## 8. Secrets

One function turns a name into a value (`app/infra/secrets.py`), fetched at startup, never per
request — so moving to a secret manager is one edit, not a search for every `os.getenv`. `.env`
is gitignored; `.env.example` is committed with names and no values.
→ `control.ai.api.secrets_not_committed`

## 9. Two things to switch off in production

`docs_url=None, redoc_url=None` outside dev. An interactive console issuing real requests
against a real agent is a cost and a data-exposure surface, not documentation; publish the
generated `contracts/openapi.json` instead.

## 10. What this adapter deliberately does not cover

Router organisation, dependency-injection style, async versus sync, ORM choice, and anything
about a frontend beyond the two rules in `16-service-and-edge.md` — never ship a provider
credential to the browser, and show the approver what `07-hitl.md` requires. None of that is
agent-specific, all of it is contested, and none of it is verifiable from an artifact.
