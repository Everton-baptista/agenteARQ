# 06. Exit codes (normative)

An implementation MUST use these codes and MUST NOT reuse them for other conditions.

| Code | Meaning | Produced by |
|---|---|---|
| 0 | success | any command |
| 1 | usage error, internal error, or version incompatibility | any command |
| 2 | structural validation failed | `validate`, `sync` |
| 3 | generated files are out of date | `sync --check` |
| 4 | a blocker-severity control failed | `check` |
| 5 | a waiver is invalid or has expired | `check` |
| 6 | a revalidation trigger fired without revalidation | `diff --strict`, `check` |

## Why they are distinct

They are distinct because continuous integration must be able to route them differently, and
because collapsing them teaches people to ignore all of them.

"You forgot to run sync" (3) is a thirty-second fix by whoever opened the pull request. "Your
agent is unsafe" (4) is a design conversation. "Your exception lapsed" (5) belongs to the person
who took it, and alarming the whole team about it makes the next person less likely to record
one honestly. "This needs revalidating" (6) is not a defect at all — it is the standard saying
that prior evidence no longer describes the system.

A tool that returns 1 for all of these is a tool whose failures get a blanket `|| true`.

## Precedence

When more than one condition holds, an implementation MUST report the **lowest** applicable
code above 0, evaluated in this order: 1, 2, 3, 5, 4, 6.

Waiver problems (5) precede a blocked gate (4) deliberately. A lapsed waiver usually explains
the blocker, and reporting the blocker first sends the reader to fix something that is already
tracked.

## What is not an error

- A `major`, `minor` or warn-mode finding. These are reported and exit 0. A gate that fails on
  everything is a gate that gets switched off.
- A baselined failure. It is reported as debt and exits 0.
- A control that does not apply to the agent. It is skipped, and an implementation SHOULD NOT
  print it by default.
- A stale translation. It is a warning unless the caller asked for strictness, because a project
  that installed the standard did not write the translations and cannot fix them.

## Errors inside checks

If a control's expression cannot be evaluated — a parse failure, a limit exceeded — the
implementation MUST treat it as an error and MUST NOT treat it as a failed or passed check. It
contributes to exit 4 alongside blockers. A control that silently reports false because it was
malformed is worse than one that is absent, because it is counted as coverage.
