# 16. The service and the edge

An agent reaches production as a service: a caller, a request, a log pipeline, an environment file, and
sometimes a web client. This standard covers that boundary — and it exists because **it is where the
controls in the other standards stop being true**.

Every rule here is jurisdiction-neutral and framework-neutral. `adapters/fastapi.md` shows one
concretely; nothing in this file names a framework.

## 0. When these rules apply

Five of the seven ask about a service, and are skipped on an agent whose manifest declares no
`interface` — `core_transport_separated`, `caller_identified`, `request_logging_redacted`,
`budget_per_caller` and `contract_generated`. A CLI tool or a batch job has no caller to identify
and no access log to redact, and reporting otherwise is noise that teaches people to stop reading
findings. A skipped control counts as neither a pass nor a failure.

**Two are not skipped, ever**: `secrets_not_committed` and `no_client_side_model_access`. They read
the repository rather than the interface, and a committed credential is public in a library, a batch
job and a service alike. This is also why they are the only two here with no grace period — the key
is already exposed, and a control that quietly stopped running on the projects that needed it would
be the same defect as noise, with the sign reversed.

## 1. Rules

### control.ai.api.core_transport_separated

**Intent.** Code that runs the agent — the loop, prompts, tools, guardrails — must not depend on the
transport that delivered the request. No request object, no header, no web framework import.

**Severity** major · **enforced_from** content 1.2 · **fail mode** fail_closed

**Why.** The drift is gradual and always locally reasonable: a handler needs one header, so the runner
imports the request; a few weeks later the agent cannot be run from a test, a queue worker, or a
backfill script, and every change has to be made through a web server. It is also the rule that makes
every other rule here testable, because a core with no transport can be exercised without one.

**How to verify.** Declare the layers as globs in `agentarch.yaml`. The check reads import statements
under `layout.paths.core` and fails on any that reference a symbol declared under
`layout.paths.edge`, or a package in the edge's dependency set.

**How to fix.** Move what the handler needed into a value the core defines. If the loop needs the
caller's identity, the core declares the identity type and the transport constructs one.

**Limits, stated.** The check reads imports textually. It does not resolve dynamic imports and will not
catch a violation routed through a string. It catches the mistake people make; a check that claimed to
prove absence would be worse than one that admits its reach.

### control.ai.api.caller_identified

**Intent.** The caller is identified from a verified credential, and every tenant or scope value is
derived from that identity server-side.

**Severity** major · **enforced_from** content 1.2 · **fail mode** fail_closed

**Why.** `05-memory-and-state.md` requires a `scope_key`. It does not say where the value comes from,
and if it comes from the request body then two tenants share one memory store while every declared
control passes. A scope the caller can set is not a scope; it is a parameter.

**How to verify.** `interface.caller.identified_by` is declared, and every agent whose
`context.memory.kind` is `user` or `shared` has a `scope_key` referencing
`interface.caller.tenant_claim`.

**How to fix.** Remove the identity fields from the request schema. A field that was never accepted
cannot be overridden.

### control.ai.api.request_logging_redacted

**Intent.** Request and response content is not logged, and the redaction list is declared.

**Severity** major · **enforced_from** content 1.2 · **fail mode** fail_closed

**Why.** This is where `capture_content: false` either holds or is a lie. A web framework logs request
paths by default and an unhandled error handler records the exception with whatever was in scope. It is
the most common route by which personal data reaches a log aggregator with a multi-year retention and
no subject-access path, and it happens on the day the service ships, silently, because the log looks
normal.

**How to verify.** `interface.logging.capture_request_body` is `false` unless
`privacy.capture_content` is explicitly true, and `interface.logging.redact` is non-empty.

**How to fix.** Log the route template rather than the concrete path — an identifier in a path is
personal data in a great many services. Log the request id, method, status, duration and tenant.
Disable the framework's own access log. Send tracebacks to an error tracker, which is a system with an
owner and a retention policy; an access log is not.

### control.ai.api.budget_per_caller

**Intent.** At least one bound is declared per caller, not only per run.

**Severity** minor · **enforced_from** content 1.2 · **fail mode** fail_warn

**Why.** `max_steps`, `max_tool_calls` and `usd_per_run` bound one run. A caller who issues ten
thousand runs is inside every one of them, and the first symptom is an invoice.

**How to verify.** `autonomy.budget` declares `runs_per_caller_per_day` or `usd_per_caller_per_day`.

**How to fix.** Enforce it where the caller is known — the same layer that resolved the identity. Return
the status code your protocol uses for rate limiting, with a retry hint.

### control.ai.api.contract_generated

**Intent.** The machine-readable interface contract is generated from the manifest, not written
alongside it.

**Severity** minor · **enforced_from** content 1.2 · **fail mode** fail_warn

**Why.** Two hand-maintained descriptions of one interface diverge, and the consumer reads the one that
is wrong. The same reasoning as `.mcp.json` being derived from the allowlist: the reviewed document is
the source, the machine file is the derivative.

**How to verify.** The file at `layout.paths.contract` exists and records a source digest matching the
manifest's `interface` block.

**How to fix.** `agentarch sync`. Edit the manifest, never the generated file.

### control.ai.api.secrets_not_committed

**Intent.** No environment file carrying secret values is tracked in version control, and a committed
example declares names without values.

**Severity** blocker · **enforced_from** immediately · **fail mode** fail_closed

**Why.** Invariant 3 says a secret is referenced by name and never carried by value. The single most
common way that breaks is `git add -A` on a day somebody is in a hurry. A credential in git history
outlives the commit that removed it.

**Why immediate rather than warned.** Every other control here enters in warn mode so that nothing
which passes today starts failing on upgrade. This one describes a credential that is already exposed,
and a grace period on that is not a kindness.

**How to verify.** No tracked file matches the environment-file patterns; an example file exists; no
file under the declared layout contains a value matching a credential pattern.

**How to fix.** Rotate the credential first — it is public. Then remove the file, ignore it, and commit
an example with names only.

### control.ai.api.no_client_side_model_access

**Intent.** A web client never holds a model provider credential and never calls a provider directly.

**Severity** blocker · **enforced_from** immediately · **fail mode** fail_closed

**Why.** A credential shipped to a browser is a public credential, and every guardrail in this standard
is on the server side. A client calling the provider directly has no input check, no output check, no
tool authorisation, no budget, and no audit trail — it is an unmetered proxy to your account with your
users' names on it.

**How to verify.** No file under `layout.paths.client` imports a provider SDK or references a provider
credential name.

**How to fix.** Route it through your own endpoint. The client sends a question and receives an answer.

## 2. Do / don't

| Do | Don't |
|---|---|
| derive the tenant from a verified credential | accept a tenant, customer or subject field in a request |
| pin model parameters in the manifest | let a caller send `model`, `temperature`, `system` or `max_steps` |
| log the route template, the status and the tenant | log the body, the query values, the answer, or an exception message |
| put the guardrails in the agent core | implement them as transport middleware — you lose them when the transport changes, and they never see the tool call |
| park a pending approval with a TTL, a tenant check and single use | block a worker thread on a human decision |
| resolve secrets once, at startup, through one function | read the environment wherever a credential is needed |
| make readiness and liveness different checks | let a temporarily unready replica be killed and lose its pending approvals |
| declare the layout and let the check read the dependency direction | rely on directory names to enforce architecture |

## 3. Affected artifacts and fields

| Artifact | Fields |
|---|---|
| `agent.yaml` | `interface.transport`, `interface.caller.{identified_by,tenant_claim}`, `interface.logging.{capture_request_body,redact}`, `interface.routes[]`, `autonomy.budget.{runs_per_caller_per_day,usd_per_caller_per_day}` |
| `agentarch.yaml` | `layout.preset`, `layout.paths.{edge,core,domain,infra,client,contract}` |
| generated | the contract at `layout.paths.contract` |
| repository | the ignore file, and the committed environment example |

## 4. Expected evidence

- the generated contract, with a digest matching the manifest
- a test asserting an unauthenticated call is refused
- a test asserting the request schema has no identity or model-parameter fields
- a log sample from a real request, showing no content
- for a client: a build output containing no provider credential reference

The first three are cheap and worth having as tests rather than attestations. A reviewer confirming that
a log contains no content is looking at one log line; a test that asserts it holds for every route.

## 5. Observed anti-patterns

**The tenant in the body.** `{"question": "...", "tenant_id": "acme"}`. Every control passes; any caller
reads any tenant.

**The access log that logs everything.** Default framework configuration, a customer's question in a
log aggregator, discovered during a subject-access request eighteen months later.

**The guardrail as middleware.** Input checked at the edge, output never checked, tool calls never
checked, and all of it lost when the framework is replaced.

**Approval by `input()`.** Works in a terminal. In a service it blocks a worker until the request times
out, and the action is neither performed nor refused.

**The approval id as a bearer capability.** No tenant check, so anyone holding an id can approve an
action they were never shown. Usually found because the ids are sequential.

**One replica's memory as the approval store.** Correct in development. With two replicas, an approval
raised by one is a 404 at the other and the run is silently lost.

**`.env` in the first commit.** The credential is rotated, the file is deleted, and it stays in the
history.

**The provider key in the frontend bundle.** Shipped to every visitor. Usually found by someone else.

**Readiness that always says yes.** A replica with no credential fails every request and looks healthy,
so the load balancer sends it everything.

## 6. External references

- OWASP Top 10 for LLM Applications — LLM02 (sensitive information disclosure), LLM06 (excessive
  agency), LLM10 (unbounded consumption). Mapped in `references/owasp-llm.md`; `reviewed_at: 2026-07-29`
- OWASP API Security Top 10 — API1 (broken object level authorization), API4 (unrestricted resource
  consumption). `reviewed_at: 2026-07-29`
- OpenTelemetry semantic conventions for GenAI, and for HTTP spans. Version pinned in the manifest;
  `reviewed_at: 2026-07-29`

This standard cites no law. Legal obligations about logging personal data live in the `reg.*` packs,
which have their own review cycles.
