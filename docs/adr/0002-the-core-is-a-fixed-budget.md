---
adr: 0002
title: The core is a fixed budget, not a list
status: accepted
date: 2026-07-28
---

# ADR 0002 — The core is a fixed budget, not a list

## Context

The L0 core is inlined into every assistant instruction file, so it is loaded on every session by
every assistant. Content that is always loaded competes with the task the user actually asked
about.

Every rule feels important to whoever wrote it, and instruction files in the wild grow
monotonically until assistants weigh each line less.

## Decision

The concatenated core must not exceed 12288 bytes, and per-file limits cap the invariants,
vocabulary terms and routing rows. Exceeding any of them **fails the build**.

Adding an invariant therefore requires removing one or demoting it to a standard.

## Alternatives considered

**A guideline with a warning.** Warnings are ignored, and this failure is silent — nobody notices
an assistant weighing instructions less.

**Per-file limits only.** Files would multiply.

**No limit; trust the authors.** This is what every instruction file in the wild does, and the
outcome is uniform.

## Consequences

**Easier.** "What is truly non-negotiable" becomes a scarce, contested decision. Fifteen
invariants everyone has argued about beat forty nobody has read.

**Harder.** A genuinely important rule sometimes cannot go in the core, and lives in a standard
loaded on demand. That is the trade being made deliberately.

**Ongoing.** Every core change is a negotiation. That is the mechanism working, not friction to
remove — and the temptation to raise the number will recur, which is why the number is in the
spec rather than in a config file.
