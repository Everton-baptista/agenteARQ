---
name: agentarch-refactor
description: >-
  Refactor an existing agent codebase to the agentarch standard, in verifiable slices, with tests
  written before each change and the gate run after it. Use when the user asks to refactor,
  restructure or "apply best practices" to agent code that already exists; after `agentarch start
  --refactor`; or when a legacy project has been adopted and somebody wants to close the debt. Not
  for describing what exists — that is `--adopt` and it has already run. Not for building
  something new — that is agentarch-new-agent.
---

# Refactoring to the standard

You are changing code somebody depends on. The standard is not the goal; a system that still
works and now also holds is the goal, and those two can diverge at any step.

**The one rule this whole procedure exists to enforce:** never change behaviour and structure in
the same commit. When something breaks afterwards — and something will — that rule is what makes
the difference between reading one diff and bisecting fifty.

## 1. Measure before touching anything

```bash
agentarch check --profile standard --format json > /tmp/before.json
agentarch check --profile standard --adopt-baseline
```

The baseline records what was already failing. Without it there is no way to prove later that the
refactor improved anything rather than moving the problem somewhere the gate does not look.

Read `/tmp/before.json` and count. Say the number out loud in your first message: *"37 controls
failing, 9 blockers."* A refactor that starts without a number ends without one.

## 2. Cover before changing

Whatever you are about to move, write a test for what it does **now**, while it still does it.

This is not optional and it is not the same as "there are tests". The question is narrower: is
*this* behaviour, the one this slice moves, asserted anywhere? If not, assert it first, in its own
commit, and watch it pass against the unmodified code.

Refactoring without that is rewriting, and the difference only becomes visible in production.

Where the codebase has no test harness at all, building one is the first slice. Skip to §4 and
treat "tests can run" as the first gate.

## 3. Order the work by what the gate says, not by what looks untidy

```bash
agentarch check --profile standard --explain-resolution
agentarch explain <control.id>
```

`explain` gives the reasoning and the remediation for each control, and which pack imposed it.
That is the order: blockers first, then majors, then the rest.

Resist reorganising directories because the layout offends you. The layout that matters is the
one `standards/16-service-and-edge.md` describes, and `AA-DEP-019` checks — the agent core must
not import the transport. Everything else is taste, and taste spends the review budget that the
real findings need.

Use `references/slicing.md` for how to cut the work when a change looks too big to do at once.

## 4. One slice, four gates

For every slice, in this order, before moving on:

```bash
<the project's own test command>          # they pass
agentarch check --profile standard        # no worse than /tmp/before.json
agentarch validate                        # artifacts still consistent
git status --porcelain                    # nothing unexpected staged
```

And the one that is not negotiable:

```bash
git diff --cached --name-only | grep -E '\.env$|credentials|\.pem$' && echo REFUSE
```

`control.ai.api.secrets_not_committed` is a blocker with no grace period, and the reason is that
by the time anybody notices, the credential is already public. If a secret appears in a diff,
stop, remove it from history, and rotate it. Do not continue the refactor around it.

## 5. Close the ratchet

```bash
agentarch check --profile standard --update-baseline
```

This removes what is now passing. It never adds — the baseline only turns one way, so the debt
disappears because it was paid rather than because it was forgiven.

Report what closed and what did not, with the number from §1: *"37 → 12, all 9 blockers closed."*

## What this procedure will not do for you

It does not make the refactor correct. Four gates per slice catch a regression the tests cover, a
control that got worse, and a leaked secret. They do not catch a behaviour nobody tested, and they
do not catch a design that satisfies every control and still serves the user badly.

When you are unsure whether a change is faithful, say so and stop, rather than continuing and
reporting a green gate. A green gate on a broken refactor is the most expensive artifact in this
whole standard.
