# Writing a control

## Before you write anything

**Can it fail?** Check that a realistic project could violate it and that the check would catch
that. A control re-checking something a schema already guarantees can never fire, and a catalogue
of those looks stronger than it is.

This has happened here once. `irreversible_requires_approval` originally re-checked the approval
block that the tool schema already requires, so it could never fail on a valid project. It now
checks what the schema cannot see: an L2 agent wiring that tool up with `approval: none`. The git
history has the change.

**Is it one rule?** If the intent needs "and", it is usually two controls, and splitting them now
means a finding points at one thing rather than at a category.

**Would you defend it at 5pm on a Friday?** A control that blocks a release has to be worth the
argument it will eventually cause.

## The fields that carry the weight

**`intent`** — what goes wrong without it, in one sentence, in terms of consequence. Not "tools
should declare permissions" but "assume the tool will be called with attacker-chosen arguments;
the only useful question is what is reachable from there".

**`remediation`** — what to do, specifically. A finding without one trains people to ignore
findings, and that habit generalises to the findings that mattered.

**`check.expr`** — restricted language only. Two mistakes are easy to make:

- `exists()` is a **reducer**. Inside `all()` it collapses the multi and answers "is this
  collection non-empty" instead of "does each element have this". Use `!= null` to stay
  element-wise.
- `[]` over a missing key is an **empty multi**, so `all()` is vacuously true. That is correct — a
  tool control must hold for an agent with no tools — but check it is what you meant.

## Severity

Start at `major`. Move to `blocker` when you can say what an attacker or an accident reaches if
the control is absent.

Every new control enters a pack with `enforced_from` at least one minor ahead, so it runs in warn
mode first. No control is born blocking. A rule that starts failing builds on the day it ships
does not get anything fixed; it gets the gate switched off, and takes the rest with it.

## Evidence

`manifest_field` alone means the control verifies a declaration. That is legitimate —
`owner_defined` is exactly that and should block — but a pack whose blockers are *all*
declarations is a form to fill in, and a test enforces that from three blockers up.

## The two artifacts

A control needs prose and a check, sharing an id. `AA-DOC-008` fails either without the other.

And it needs a **failing example** under `examples/99-failing/`, with an `expected.yaml` saying
which command rejects it and with which exit code. A control nobody has watched fail is a control
nobody knows works — and a refactor that quietly stops enforcing it looks exactly like a green
build.
