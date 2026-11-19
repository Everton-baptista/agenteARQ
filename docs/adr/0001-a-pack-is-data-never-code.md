---
adr: 0001
title: A pack is data, never code
status: accepted
date: 2026-07-28
---

# ADR 0001 — A pack is data, never code

## Context

Controls need checks, and checks need expressions. The options were an embedded scripting
language, a policy language such as Rego, or a restricted expression language written here.

Packs are meant to travel: through a registry, from third parties, into build pipelines that run
with credentials.

## Decision

`check.kind` has exactly four values and none executes anything. Expressions use a restricted
language, specified in `spec/normative/04`, with no `eval`, no imports, no arbitrary calls, a
closed function set, and hard limits on length, depth, cardinality and steps.

An implementation may not add a check kind that runs a program and still claim compliance.

## Alternatives considered

**Embed a scripting language.** Most expressive, and it makes installing a pack a decision about
whose code you run. A governance tool that executes third-party code to verify governance does not
hold up.

**Rego, or another policy engine.** Genuinely designed for this and battle-tested. Rejected for
two reasons: it is a large dependency in a tool whose argument is minimal trust, and it is a
language adopters must learn before they can read a control. These controls need caution far more
than they need power.

**No expressions; hard-code every check.** Safest, and it makes a pack something only this
repository can produce — ending the registry and most of the reason to call it a standard.

## Consequences

**Easier.** Installing a pack from a stranger is a decision about which rules you accept, not
about whose code you run. The archive rules follow from the same principle: only `.yaml` and
`.md`, no traversal, no symlinks, checksum verified before anything is written.

**Harder.** Some checks cannot be expressed and never will be. `manual_attestation` exists for
those, and it is an honest record rather than a gap disguised as coverage.

**Ongoing cost.** Every request to add a function is weighed against the guarantee. The answer
will usually be no, and the reason has to be given each time.
