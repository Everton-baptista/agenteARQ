---
rfc: 0001
title: Cover the service that carries the agent — layout, contract, edge
status: draft
author: Everton Baptista
created: 2026-07-29
affects: [spec, content, cli]
---

# RFC 0001 — Cover the service that carries the agent

## 1. Problem

Every blueprint ships `app/agent.py` and is run as `python app/agent.py "question"`. Nobody
delivers that to a customer. The thing that reaches production is a service: an HTTP API, an
authenticated caller, a request body, a log pipeline, an environment file, and — often — a web
client.

That gap is not cosmetic, because **it is where the controls that already exist stop being
true**:

- `privacy.capture_content_default_off` is declared in the manifest. The prompt then leaks
  through the access log, because the web framework logs request bodies by default and nothing
  in agentarch has an opinion about that.
- `agent.memory_scoped` declares `scope_key`. The value for that key comes from the caller's
  identity, which lives in the transport layer the standard does not describe. Two tenants share
  a memory and every control passes.
- `agent.budget_bounded` bounds a run. A caller who issues ten thousand runs is inside every
  declared budget.
- Invariant 3 says a secret is referenced by name, never by value. The single most common way
  that breaks is a committed `.env`, which nothing checks.
- Invariant 2 says untrusted content goes in a delimited block as data. In the blueprints the
  untrusted content arrives on `argv`. In a service it arrives as a JSON body, and the
  concatenation that invariant 2 forbids is one f-string away in a route handler.

There is also a plainer failure: the manifest is described as the contract, and there is no
artifact anywhere that says what the service accepts and returns. `01-agent-contract.md` was
supposed to cover typed I/O — the word "typed" does not appear in it, and the manifest schema has
no field for it. A consumer of the API has nothing to read and nothing to validate against.

Concretely, today: a team installs agentarch, reaches conformance L3, wraps the agent in FastAPI
in an afternoon, logs every request body to CloudWatch with the customer's personal data in it,
serves two tenants from one memory store, and ships the provider API key to the browser. Every
control still passes. **The gate is green and the product is unsafe** — which is the definition of
conformance theatre the design set out to avoid.

## 2. Proposed rule

Five rules, stated as they would appear in a standard:

1. **The agent core must not depend on the transport.** Code that runs the agent loop, prompts,
   tools and guardrails must not import the web framework, the request object, or anything else
   that only exists because the caller arrived over HTTP.
2. **The service interface is declared in the manifest and the machine-readable contract is
   generated from it**, never written by hand alongside it.
3. **The caller is identified, and the tenant scope is derived from that identity server-side.**
   A scope key that can be set by the caller is not a scope.
4. **Request and response content is not logged by default**, and the redaction list is declared.
5. **Secrets reach the process by reference from the environment.** The environment file is not
   committed; a committed example declares the names and never the values.

And one rule for the web client, only when there is one:

6. **The browser never holds a model provider credential and never calls a provider directly.**

## 3. How it is verified

This is the section that decides whether the RFC is real. Layout conventions are famously
unverifiable, so the proposal does not verify layout — it verifies **declared paths and the
dependency direction between them**.

`agentarch.yaml` gains a `layout` block with a default:

```yaml
layout:
  preset: service          # service | library | cli | custom
  paths:
    edge:    ["api/**"]        # transport: routes, middleware, dependencies
    core:    ["agent/**"]      # the agent loop, prompts, tools, guardrails
    domain:  ["domain/**"]     # business logic: no LLM, no HTTP
    infra:   ["infra/**"]      # database, cache, external clients
    client:  ["web/**"]        # frontend, when there is one
    contract: "contracts/openapi.json"
```

A project with a different but equally valid layout changes the globs. That is what keeps this
from being a Python-shaped rule imposed on a Spring project — the rule is the direction of the
arrows, not the spelling of the directories.

| Control | `check.kind` | What it reads |
|---|---|---|
| `control.ai.api.core_transport_separated` | `static_manifest` + lint | import graph: no path under `core` may import a symbol declared in `edge`, nor a framework named in the project's `edge` dependency set |
| `control.ai.api.contract_generated` | `file_exists` + hash | `layout.paths.contract` exists and its `x-agentarch-source-sha256` matches the manifest's `interface` block |
| `control.ai.api.caller_identified` | `static_manifest` | `interface.caller.identified_by != null`, and every agent with `context.memory.kind` in `user, shared` has `scope_key` referencing `interface.caller.tenant_claim` |
| `control.ai.api.request_logging_redacted` | `static_manifest` | `interface.logging.capture_request_body == false` unless `privacy.capture_content` is explicitly true, and `interface.logging.redact` is non-empty |
| `control.ai.api.budget_per_caller` | `static_manifest` | `autonomy.budget` declares at least one of `runs_per_caller_per_day`, `usd_per_caller_per_day` |
| `control.ai.api.secrets_not_committed` | lint | no tracked file matches `.env` (bare), `.env.local`, `.env.production`; `.env.example` exists; no `layout.paths` file contains a value matching the provider key patterns |
| `control.ai.api.no_client_side_model_access` | lint | no file under `layout.paths.client` imports a provider SDK or references a provider key name |

The import lint (`AA-DEP-019`) is the load-bearing one and the one that could be wrong, so it is
worth stating its limits: it reads import statements textually per language, it does not resolve
dynamic imports, and it will not catch a violation smuggled through a string. It catches the
mistake people actually make — a route handler imported into the agent runner to read
`request.headers` — and it fails closed only on what it can see. A lint that pretends to prove
absence would be worse than one that admits its reach.

Two rules from §2 are **not** proposed as controls, deliberately:

- **The untrusted-input boundary at the edge** is already `genai.untrusted_content_isolation`.
  It needs prose naming the route handler as the place it is usually broken, not a second control
  with a different id. Duplicating a control to cover a new location is how a catalogue becomes
  unmaintainable.
- **What the approval screen must show** stays `manual_attestation` under `07-hitl.md`. Whether a
  human can understand a refund preview is not decidable by a linter, and pretending otherwise
  would be exactly the LLM-judge-as-gate mistake in another costume.

## 4. Severity and grace period

| Control | Severity | `enforced_from` |
|---|---|---|
| `secrets_not_committed` | blocker | immediate |
| `no_client_side_model_access` | blocker | immediate |
| `core_transport_separated` | major | content 1.2 |
| `caller_identified` | major | content 1.2 |
| `request_logging_redacted` | major | content 1.2 |
| `contract_generated` | minor | content 1.2 |
| `budget_per_caller` | minor | content 1.2 |

The two blockers are immediate because both describe a credential already exposed — a grace
period on "your API key is in the repository" is not a kindness. Everything else enters in warn
mode. Nothing here has an `enforced_from` earlier than content 1.2, so no project that passes
today starts failing on upgrade.

## 5. Cost of adoption

For a project that already uses agentarch and already has a service: declare `layout` (five
globs, ten minutes), declare `interface` (thirty minutes, most of it deciding what the caller
identity actually is), generate the contract (`agentarch sync`), and set
`capture_request_body: false` in one place.

The expensive one is `core_transport_separated`, because a project that grew the agent inside the
route handler has to move code. That is the whole point of the control and the reason it gets two
minors of warning rather than one.

For a project starting from a blueprint: zero. The blueprint ships in the shape that passes.

## 6. What happens to existing adopters

Nothing breaks on upgrade. `agentarch/project/` is never overwritten, the two blockers only fire
on a genuinely exposed credential, and everything else warns until content 1.2.

The visible change is that **the blueprints change shape**: `app/` becomes `api/` + `agent/` +
`domain/`. That affects nobody's installed project — a blueprint is copied once, at install — but
it does mean the README, the four `app/README.md` files and the closing text of `start` all
change together, or the documentation describes a tree that no longer exists.

## 7. Alternatives considered

**Do nothing.** The status quo is a standard that governs the agent and ignores the service that
carries it, while claiming to make agents production-ready. The five failure modes in §1 are all
reachable with a green gate. Not good enough.

**Publish a folder convention as prose, with no controls.** This is what was asked for first, and
it is the tempting version because it is a day of work. It fails the project's own admission test:
prose with no verifiable consequence does not enter the standard. It would also be the first
section anyone skips, because a layout you cannot check is advice, and there is better advice than
mine elsewhere.

**Put the FastAPI layout in the standard directly.** Rejected by `AA-FWK-014`, and rightly. The
moment `16-*.md` names FastAPI, the standard needs an opinion for Express, Spring and ASP.NET, and
a .NET team reading a Python-shaped rulebook concludes the project is not for them. The layout
lives in `content/adapters/fastapi.md` and in the blueprints, where naming a framework is the
whole point.

**Verify the layout by matching directory names.** Rejected because it is wrong more often than
right. Every project that uses `src/`, `internal/`, `lib/` or a monorepo package boundary would
fail a rule that describes taste rather than a property. Declared globs plus a dependency
direction is checkable and does not care about spelling.

**Cover general backend and frontend practice** — ORM choice, migrations, state management, CSS
architecture. Rejected: none of it is agent-specific, all of it is contested, none of it is
verifiable from an artifact, and the plan names this exact drift as what kills portability. What
survives the cut is the short list in §2, every item of which exists *because* an agent is
involved.

## 8. Prose

New standard: `content/standards/{en,pt-BR}/16-service-and-edge.md`, following the mandatory
skeleton (Rules · Do/don't · Affected artifacts and fields · Expected evidence · Observed
anti-patterns · External references).

New adapter: `content/adapters/fastapi.md`, answering the same five questions every adapter
answers, plus the layout. This is where FastAPI is named, and the reference tree is:

```
api/                        # thin. Transport only.
  main.py                   #   app factory; middleware order is explicit and commented
  deps.py                   #   caller identity, tenant scope, per-caller budget
  routes/                   #   one module per resource; handlers do no agent work
  schemas/                  #   request and response models — generated contract's source
  middleware/               #   redacted logging, tracing, rate limit
agent/                      # the agent. Imports nothing from api/.
  runner.py                 #   the loop, bounded by the manifest
  prompts/                  #   versioned, hashed
  tools/                    #   one module per .tool.yaml
  guardrails/               #   input, output, action
domain/                     # business logic. No LLM, no HTTP.
infra/                      # database, cache, provider clients
web/                        # frontend, only if there is one
contracts/openapi.json      # GENERATED — agentarch sync
tests/
.env.example                # committed: names, never values
.env                        # gitignored
```

Existing standards that gain a cross-reference rather than new text: `02-prompt-and-context.md`
(the route handler is where untrusted content gets concatenated), `05-memory-and-state.md` (the
scope key comes from the caller), `10-privacy.md` (the access log is where content capture
actually happens), `12-observability.md` (the agent span is a child of the request span),
`13-resilience-and-cost.md` (per-caller budget).

Schema change: `interface` in `agent.manifest.schema.json`, and `layout` in
`config.schema.json`.

New sync target: `openapi` → `contracts/openapi.json`, derived from `interface` exactly as
`mcp_json` is derived from the allowlist. The reviewed document is the source; the machine file is
the derivative.

## 9. Sequence

Deliberately code-first. A standard written before the code it governs produces controls nobody
can satisfy, and the fastest way to discover that a control is wrong is to try to pass it.

| Step | What | Done when |
|---|---|---|
| 1 | `rag-support` rebuilt in the reference tree with a working FastAPI service — routes, `deps.py` with caller identity and tenant scope, redacted logging middleware, `.env.example`, `contracts/openapi.json` written by hand for now | `uvicorn` serves it, a request with a bearer token gets a grounded answer with citations, and the access log shows a redacted body |
| 2 | The other three blueprints follow the same tree; `app/README.md` and the closing text of `start` rewritten | all four install, run and pass `check` |
| 3 | `content/adapters/fastapi.md` extracted from step 1 — the code already exists, so the adapter documents rather than invents | the five adapter questions answered with code copied from a blueprint that runs |
| 4 | `16-service-and-edge.md` in `en`, then `pt-BR` with `source_sha256` | `AA-DOC-008` passes both directions against the controls in step 5 |
| 5 | The seven controls, `interface` and `layout` schemas, `AA-DEP-019` lint, `openapi` sync target | the two blockers fail a project with a committed `.env`; the five majors and minors warn |
| 6 | `examples/99-failing/` cases: committed `.env`, provider key in `web/`, `agent/` importing `api/`, contract out of sync | each exits with the documented code |

Steps 1 and 2 have standalone value and change no schema, so they can ship in a patch release.
Steps 3–6 are a content minor and go through this RFC's acceptance.
