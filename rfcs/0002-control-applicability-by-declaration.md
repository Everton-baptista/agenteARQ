---
rfc: 0002
title: Skip a control the agent's shape puts out of reach
status: draft
author: Everton Baptista
created: 2026-07-30
affects: [spec, content, cli]
---

# RFC 0002 — Skip a control the agent's shape puts out of reach

## 1. Problem

`examples/99-failing/unpinned-model` is a project with one agent, no HTTP interface, and one
defect: a floating model alias. Running the gate on it reports five findings.

```
control.ai.api.budget_per_caller          note
control.ai.api.caller_identified          note
control.ai.api.core_transport_separated   note
control.ai.api.request_logging_redacted   note
control.ai.supply.model_pinned            error
```

Four of the five describe a service that does not exist. There is no caller to identify, no
access log to redact, no per-caller budget to bound, and no transport to keep out of the core.
The reader is told, correctly and uselessly, that an agent nobody can call over HTTP has not
declared who may call it over HTTP.

`spec/normative/02-control-and-pack.md` §4 already rules on this: *"A control that does not apply
MUST be reported as skipped and MUST count as neither a pass nor a failure. An implementation
SHOULD NOT print skipped controls by default; an inapplicable control is noise, and noise is what
makes people stop reading output."*

So the rule exists and the machinery exists — `applies_to`, `controlApplies`, and a `Skipped`
flag that `Score` already excludes from the maturity numbers. What is missing is a condition that
can express the thing that makes these four inapplicable.

The four available conditions cannot. `autonomy_min`, `stage_min` and `processes_personal_data`
are unrelated. `system_type` looks like the answer and is not: its values are
`generative_chat`, `generative_rag`, `agentic_task`, `agentic_workflow`, `multi_agent` and
`classifier` — what the agent **is**, not how it is **reached**. Any of the six can be put behind
an HTTP interface, and a `generative_rag` agent behind an API needs `caller_identified` exactly as
much as an `agentic_task` one does.

This is not only noise. It is what broke CI: the SARIF assertion in
`.github/workflows/ci.yml` read the first result of a blocked project and found a warn-mode
`note` about an interface the project does not have.

## 2. Proposed rule

A control may declare `applies_to.declares`, naming the optional manifest sections it needs in
order to have anything to say. The control is evaluated only when **every** named section is
present and non-empty in the agent's manifest; otherwise it is skipped, and counts as neither a
pass nor a failure.

## 3. How it is verified

This is not a new check kind and adds nothing to the expression language. It is a fifth condition
in `applies_to`, evaluated by the same resolution step that already applies the other four,
before any expression runs.

The value is a list drawn from a **closed enum** of the manifest's optional top-level sections:

```
context · evaluation · handoff · interface · jurisdictions · languages
lifecycle · links · mcp · observability · policy · privacy · tools · users
```

Required sections (`id`, `owner`, `stage`, `system_type`, `purpose`, `out_of_scope`, `autonomy`,
`model`, `prompts`, `guardrails`) are deliberately excluded. They are always present, so naming
one produces a condition that can never skip — a control that looks conditional and is not.

The enum is closed for the same reason the expression language is restricted: a free-form path
would make `applies_to` a second, weaker place to write a check, evaluated before the real one,
with none of `04`'s guarantees about what a hostile pack cannot do. Presence of a named section
is the whole vocabulary.

`exists()` semantics are reused as `04` already defines them: present, non-null, and — for lists,
maps and strings — non-empty. A manifest carrying `interface: {}` has not declared an interface.

## 4. Severity and grace period

None. This RFC adds no control and changes no severity. It can only move a finding from reported
to skipped, never the reverse, so nothing that passes today starts failing.

It is an **additive spec minor** under `08-versioning.md`: an optional field on an optional
object, which cannot make an existing valid document invalid.

## 5. Cost of adoption

Nothing, for a project. No manifest changes, no configuration.

For a pack author, one optional line per control. The reference implementation applies it to five
controls in `api.edge`, listed in §6.

## 6. What happens to existing adopters

Nobody starts failing. Some stop being told about controls they were never able to satisfy.

Five controls in `api.edge` gain `applies_to.declares: [interface]`:

| Control | Why it needs an interface |
|---|---|
| `caller_identified` | reads `agent.interface.caller.identified_by` |
| `request_logging_redacted` | reads `agent.interface.logging.capture_request_body` |
| `contract_generated` | the contract is generated from `interface.routes` |
| `budget_per_caller` | a caller is a thing the interface defines |
| `core_transport_separated` | there is no transport to separate the core from |

The two blockers stay unconditional, and this is the load-bearing part of the proposal:

- `secrets_not_committed` reads `repository.tracked_secret_files`
- `no_client_side_model_access` reads `repository.client_provider_refs`

Neither reads the interface. A committed `.env` is a public credential in a CLI tool, a batch
job, and a library, and it is public the moment it is pushed. Narrowing these two to services
would be the failure mode this RFC exists to avoid, applied in the wrong direction: a control
silently not running on a project that needed it. They ship with no grace period precisely
because the credential is already exposed, and applicability must not quietly reintroduce one.

## 7. Alternatives considered

**Do nothing.** The noise stays, and §4 of `02-control-and-pack.md` describes a requirement the
reference implementation does not meet. It has already cost one CI outage.

**Fold the condition into `check.expr`** — `agent.interface == null or agent.interface.caller.identified_by != null`.
This needs no spec change, and is wrong. It reports a **pass**, not a skip. `Score` counts
passes, so every agent with no interface would earn five free `api` controls toward its maturity
number, and `conformance` would count evidence for an interface nobody has. Manufacturing
compliance out of absence is the specific thing this project exists to prevent; that it would
have been the cheap fix is exactly why it is worth naming here.

**Reuse `system_type` with a new enum value such as `service`.** It conflates two independent
axes. An agent is a `generative_rag` **and** exposed over HTTP; forcing a choice loses whichever
half is not chosen, and every RAG control would then skip on a RAG agent that happens to be a
service.

**A free-form path expression in `applies_to`.** More general, and it creates a second evaluation
surface reachable by third-party packs, ahead of the checks `04` was written to constrain. The
closed enum gives up generality that no proposed control needs.

## 8. Prose

`spec/normative/02-control-and-pack.md` §4 gains the condition in its list and the paragraph on
why the enum is closed. `03-resolution.md` §2 is unchanged — it governs `applies_when` on a
**pack**, which is a different mechanism with a different vocabulary, and conflating the two is a
mistake this RFC should not introduce.

No standard under `content/standards/` changes. Applicability is a property of how a control is
evaluated, not a rule an agent has to follow, so there is no prose half and `AA-DOC-008` does not
apply.
