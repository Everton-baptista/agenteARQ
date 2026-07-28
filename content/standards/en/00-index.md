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

## 6. Status of this catalogue

Published: `01`, `03`, `08`. The remaining standards named in the routing table are planned and
listed in the repository roadmap. A routing entry pointing at an unpublished standard is a known
gap, not an error — but it is a gap, and `agentarch explain` will say so rather than pretending
otherwise.
