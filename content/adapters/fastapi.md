# Adapter: FastAPI

This is the transport adapter. FastAPI is a web framework, not an agent framework, so this file
answers a different set of questions from `langgraph.md` or `claude-agent-sdk.md`: not how the loop is
structured, but where the loop is *reached from* — and the two must not be the same place.

Naming a framework is only allowed here and in the blueprints. Every rule below exists
framework-neutrally in `standards/16-service-and-edge.md`; this is what it looks like in code.

All the code here is copied from a service that runs. `content/blueprints/rag-support/` is the whole
thing, with tests.

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

**`agent/` must not import `api/`.** That is the whole layout; the directory names are just spelling.
Declare the globs in `agentarch.yaml` and `control.ai.api.core_transport_separated` checks the
direction:

```yaml
layout:
  preset: service
  paths:
    edge:  ["app/api/**"]
    core:  ["app/agent/**"]
    domain: ["app/domain/**"]
    infra: ["app/infra/**"]
    contract: "contracts/openapi.json"
```

The drift it prevents is gradual and always locally reasonable. A handler needs one header, so the
runner imports the request; a few weeks later the agent only runs inside a web server, and it can no
longer be run from a test, a queue worker, or a backfill script. `app/cli.py` in the blueprint exists
to prove the rule holds — same agent, no server.

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

`Principal` is defined in `app/agent/`, not here, and it holds no reference to the request. That is
what lets the same type be constructed by a CLI, a worker or a test.

The memory `scope_key` is derived from it:

```python
@property
def memory_scope_key(self) -> str:
    return f"{self.tenant_id}:{self.subject}"
```

A scope key the caller can set is not a scope — it is a parameter, and two tenants end up sharing one
store while every declared control still passes. → `control.ai.api.caller_identified`

## 2. The contract, and what must not be in it

```python
class AskRequest(BaseModel):
    question: str = Field(min_length=1, max_length=4000)
    conversation_id: str | None = Field(default=None, max_length=64)
```

Two absences are the security property:

- **no `tenant_id`, `customer_id` or `subject`.** A request field that names the customer is a request
  field that selects the customer. It cannot be overridden if it was never accepted.
- **no `model`, `temperature`, `system` or `max_steps`.** Those are pinned in the manifest. A caller
  who can raise `max_steps` can raise your bill; a caller who can send a system prompt owns your
  agent.

`contracts/openapi.json` is generated from the manifest's `interface` block by `agentarch sync`, the
same way `.mcp.json` is generated from the allowlist: the reviewed document is the source, the machine
file is the derivative. → `control.ai.api.contract_generated`

## 3. Where the three guardrails go

They go in `app/agent/guardrails.py` and are called from the loop — not from the route, and not from
FastAPI. A guardrail implemented as framework middleware is a guardrail you lose when you change
frameworks, and it never sees the tool call.

The route does one thing worth noting: it passes `question` and nothing else, and the loop puts it
inside a delimited block.

```python
@router.post("/ask", response_model=AskResponse)
async def ask(body: AskRequest, request: Request,
              principal: Principal = Depends(enforce_caller_budget)) -> AskResponse:
    request.state.tenant_id = principal.tenant_id      # for the log; never the content
    outcome = runner.run(body.question, principal)
    ...
```

**The route handler is where invariant 2 gets broken.** `f"Answer this: {body.question}"` in a system
prompt is one line, looks harmless, and is the whole vulnerability. If the prompt string is built
anywhere under `api/`, the boundary has already moved.

## 3b. Where the tool permission check happens

In `app/agent/guardrails.py::action_guardrail`, called from the dispatcher — never in a route handler
and never in a FastAPI dependency. Two reasons, and the second is the one people get wrong:

A dependency runs once per request. A tool runs once per call, and a single request makes several. A
permission check in `Depends()` authorises the request; the model then makes four tool calls and three
of them were never checked.

And the check must never call the model. It is the last thing that runs before something happens in
the world, and a check that shares the model's context shares its compromise.

What the transport layer contributes is the *authority* the check reads:

```python
# app/api/deps.py — resolved from the credential, never from the body
Principal(tenant_id="acme", subject="op-7", role="account_manager")
```

```python
# app/agent/guardrails.py — the check itself, with no request in sight
def action_guardrail(name, args, spec, principal, autonomy):
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"
    if not principal.may_request(spec["effect"]):
        return False, f"role {principal.role!r} may not request a {spec['effect']} action"
    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        ...
    if spec["effect"] in IRREVERSIBLE and autonomy not in ("L0_suggest", "L1_act_with_approval") \
       and spec["_approval"] not in ("human", "policy"):
        return False, f"{name} is {spec['effect']} at {autonomy} with approval: none"
    return True, ""
```

The role check runs **before** an approval is raised. Otherwise anyone can fill the approver's queue
with requests they were never allowed to make, and approval fatigue is manufactured exactly that way.

→ `control.ai.tool.least_privilege`, `control.ai.tool.irreversible_requires_approval`

## 4. Handoff, and human approval when the approver is not in the request

### Handoff

In-process, a handoff is a function call across a typed boundary
(`app/agent/handoff.py` in the multi-agent blueprint) and the transport layer never sees it. **The
caller talks to the orchestrator; which specialist is reached is decided by the manifest's declared
contracts.** So there is no `agent_id` field in the request:

```python
class AskRequest(BaseModel):
    question: str = Field(min_length=1, max_length=4000)
    # deliberately absent: agent_id, authority
```

A request that could name an agent could bypass triage. One that could name an authority could grant
itself any.

Across services it is a different problem, and worth stating because HTTP makes it look easy:

| in-process | over HTTP |
|---|---|
| the payload was composed by your code | it arrives from another process and is **untrusted again** — validate it at the receiving edge, not only at the sending one |
| the shared budget is an object passed down | it has to travel: propagate steps, depth, path and spend as headers, or each service enforces its own bound and A → B → A has none |
| authority is a parameter | it must be a *narrower* credential the callee verifies. Forwarding the caller's own token makes every hop as privileged as the first |
| the return point is the call stack | it is a declared endpoint with a timeout, and `on_timeout` decides — a handoff with no timeout is a request that hangs until something else gives up |

`return_point` and `timeout_s` in the contract are what make the second column checkable.

### Human approval

An `input()` call is not an option, and neither is blocking a worker on a person — that is how a
service falls over under the load of its own approvals. So the run pauses and returns:

```python
if outcome.status == "awaiting_approval":
    approval_id = approvals.queue.put(Pending(...))
    return AskResponse(status="awaiting_approval", approval_id=approval_id,
                       approval=outcome.approval)
```

A second request carries the decision. Four properties, each mapping to a rule in `07-hitl.md`:

| property | why |
|---|---|
| TTL on the record | `on_timeout: deny` is only true if something expires. A pending approval with no deadline gets granted at 3am by whoever is tired enough |
| tenant checked, and the key namespaced | otherwise the approval id is a capability anyone holding it can spend |
| single use | replaying an approval replays the irreversible action it authorised |
| an audit line: who, what, when, and what they were shown | an approval with no record is indistinguishable from no approval once anyone asks |

Store it behind a four-method interface (`app/infra/store.py`). In-memory is correct for one replica
and **wrong for two** — an approval raised by replica A is a 404 at replica B and the run is lost. Say
so in your README rather than leaving it to be discovered.

## 5. Telemetry, and the log that is usually the leak

```python
# app/api/middleware.py — what is recorded
log.info("request", extra={
    "request_id": request_id,
    "method": request.method,
    "route": _route_of(request),     # the template: /v1/approvals/{id}, not the id
    "status": response.status_code,
    "duration_ms": ...,
    "tenant": getattr(request.state, "tenant_id", "-"),
})
```

And what is not: the body, the query values, the answer, the token, and the exception message from an
unhandled error — which routinely contains the value that caused it.

Disable the framework's own access log. It records the concrete path, and an identifier in a path is
personal data in a great many services:

```python
logging.getLogger("uvicorn.access").disabled = True
```

This is where `capture_content: false` either holds or is a lie. It is the single most common way
personal data reaches a log aggregator with a two-year retention and no subject-access path, and it
happens on the day the service ships, silently, because the log looks normal.
→ `control.ai.api.request_logging_redacted`

Spans and metrics go in `app/infra/telemetry.py`, with the semconv version pinned and content never
an attribute — a trace backend is a log aggregator with better indexing. The cost figure that feeds
the dashboard is the same one that enforces the budget, so the bill and the limit cannot disagree.

## 6. The budget the manifest does not bound

`max_steps`, `max_tool_calls` and `usd_per_run` bound **one run**. A caller who issues ten thousand
runs is inside every one of them.

```python
def enforce_caller_budget(principal: Principal = Depends(current_principal)) -> Principal:
    ...
    if len(recent) >= DEFAULT_RUNS_PER_DAY:
        raise HTTPException(429, "daily run budget exhausted",
                            headers={"Retry-After": str(WINDOW_SECONDS)})
```

→ `control.ai.api.budget_per_caller`

## 7. Health, and why it is readiness

```python
@router.get("/health")
async def health() -> Health:
    ...
```

It verifies three things: the credential resolves, the system prompt still hashes to what the manifest
records, and the provider circuit is not open. A service serving a prompt nobody reviewed should not
receive traffic, and a service that starts without its credential and fails every request looks
healthy to a load balancer — which then sends it everything.

Readiness and liveness must be **different checks**. When the provider circuit is open the replica is
unready and must stop taking traffic, but it must not be killed: a restart loses every pending
approval it is holding.

## 8. Secrets

One function turns a name into a value (`app/infra/secrets.py`), so moving from environment variables
to a secret manager is one edit rather than a search for every `os.getenv`. Fetch at startup, never per
request. Fail at startup, never at request time.

`.env` is gitignored; `.env.example` is committed with names and no values.
→ `control.ai.api.secrets_not_committed`

## 9. Two things to switch off in production

```python
docs_url="/docs" if os.getenv("ENV") == "dev" else None,
redoc_url=None,
```

An interactive console that issues real requests against a real agent is a cost and a data-exposure
surface, not documentation. Publish `contracts/openapi.json` instead — it is generated, reviewed, and
does not execute anything.

## 10. What this adapter deliberately does not cover

Router organisation, dependency-injection style, async versus sync, ORM choice, migrations, and
anything about a frontend beyond the two rules in `16-service-and-edge.md` — never ship a provider
credential to the browser, and show the approver what `07-hitl.md` requires.

None of that is agent-specific, all of it is contested, and none of it is verifiable from an artifact.
A standard that has an opinion about `src/routers/` has to have one for Express, Spring and ASP.NET
too, and stops being portable the day it does.
