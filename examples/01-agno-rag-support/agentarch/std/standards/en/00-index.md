# 00. Index

Purpose: how this body of standards is organised and how to read it.
Version: 0.1 · Status: draft · Scope: all standards in `content/standards/`.

---

## 1. The two forms of every rule

Every rule in agentarch exists twice, under one identifier:

| Form | Lives in | Audience |
|---|---|---|
| Prose | `content/standards/<lang>/NN-*.md` | people, and the AI assistant reading the routing table |
| Executable control | `content/packs/controls/<type>/<name>.yaml` | `agentarch check` |

`agentarch validate` checks the correspondence in **both** directions. A control with no prose
section fails, and a documented rule that no control verifies fails too.

This is the mechanism that keeps the standard from becoming shelfware. Prose without a
verifiable consequence is an opinion; a check with no explanation is an obstacle. Requiring both
means every rule has to survive two different questions — *can you justify this?* and *can you
detect it?* — and a surprising number of plausible-sounding rules survive neither.

When a rule genuinely cannot be automated, it is admitted with `check.kind:
manual_attestation`. That is an honest declaration that a human asserted something, not a
loophole: the attestation is recorded, owned and dated like any other evidence.

---

## 2. Layers, and what gets loaded when

| Layer | What | Loaded |
|---|---|---|
| **L0 core** | `content/core/` — identity, invariants, vocabulary, routing | always, inlined into every assistant instruction file |
| **L1 standards** | this directory | on demand, via the routing table |
| **L2 references** | `content/references/` — external framework mappings, attack catalogues | only when a standard or skill links to it |
| **L3 machine** | schemas, controls, packs | read by the CLI, never by the model |

L0 has a **fixed byte budget** enforced by `AA-BUD-010`. Adding an invariant requires removing
another or demoting it here. That constraint is the point: an always-loaded document that grows
without limit is eventually ignored in full, which is worse than a short one that is followed.

---

## 3. Severity

| Severity | Means | Effect on `agentarch check` |
|---|---|---|
| `blocker` | the release must not proceed | non-zero exit (4) |
| `major` | must be fixed, does not block today | reported; counts against the maturity score |
| `minor` | worth improving | reported only |

At least half of all `blocker` controls must be verifiable from an **executable artifact** — an
eval result, a test, a hash — rather than from a declared field. A control that only checks
whether someone filled in a box is at most `major`. Without that rule the standard degrades
into form-filling, and a project can be fully "compliant" while nothing about it is safer.

---

## 4. Fail modes

Guardrails declare what happens when they cannot reach a verdict.

| Mode | Behaviour | Use when |
|---|---|---|
| `fail_closed` | block the action | the check is deterministic, or the severity is high |
| `fail_warn` | allow, record loudly | degradation is preferable to a hard stop |
| `fail_open` | allow | the check is probabilistic and the impact is low |

The default pairing, and the one the invariants encode: **a deterministic wall is
`fail_closed`; an LLM judge is `fail_open`** unless severity is high or critical. An
unreliable check wired to block produces irreproducible failures, and a gate that fails
irreproducibly is switched off within a week — taking the reliable checks with it.

---

## 5. Reading order

Read `01-agent-contract.md` first; everything else assumes an agent has a declared scope, an
owner and a bounded loop. After that, follow the routing table in `content/core/30-routing.md`
and read only what the task needs.

---

## 6. Glossary

The core keeps only the five terms the invariants lean on; this is the full list. Use these
words with these meanings — precision here prevents whole classes of design error.

- **Agent** — a system that chooses actions to reach a goal. If it cannot call a tool, it is a
  prompt, not an agent.
- **Manifest** — `agent.yaml`. The declared contract of one agent. Source of truth.
- **Tool** — a capability with a typed contract and declared permissions. Defined in a
  `.tool.yaml`, never only in code.
- **Effect** — what a tool does to the world: `read`, `write`, `irreversible`, `money`,
  `communication`. Drives approval and guardrails.
- **Autonomy** — `L0_suggest`, `L1_act_with_approval`, `L2_act_reversible`,
  `L3_act_irreversible_bounded`, `L4_autonomous`. How far the agent may go unattended.
- **Guardrail** — a check at one of three points: user **input**, model **output**, tool
  **action**. Not a synonym for prompt instruction.
- **Fail mode** — what happens when a guardrail cannot decide: `fail_closed` (block),
  `fail_warn` (allow, record), `fail_open` (allow).
- **Untrusted content** — anything not authored by your code: user text, retrieved documents,
  web pages, tool results, other agents' output. Always data, never instruction.
- **Control** — one verifiable rule, `control.ai.<type>.<name>`. Has prose and an executable check.
- **Pack** — a versioned set of controls with severities. Data, never code.
- **Profile** — which packs apply here: `minimal`, `standard`, `regulated`.
- **Gate** — `agentarch check`. Blocks a release on `blocker` severity.
- **Waiver** — a time-boxed, owned exception. Maximum 90 days, never permanent.
- **Handoff** — one agent transferring work to another, with typed payload, declared authority,
  return point and timeout.
- **Revalidation trigger** — a change that invalidates prior assurance: model, system prompt,
  RAG corpus, provider, new tool, raised autonomy, disabled guardrail, new MCP server.
- **Evidence** — an artifact proving a control holds: eval result, test, hash, attestation.
  Distinct from a declaration.
- **Shim** — a generated assistant instruction file (`AGENTS.md`, `CLAUDE.md`, …). Output, never input.

---

## 7. Status of this catalogue

Published: `01`–`16`. Every routing entry points at a real file. If one ever points at a
planned standard, that is a known gap, not an error — and `agentarch explain` will say so
rather than pretending otherwise.
