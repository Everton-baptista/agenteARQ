# Vocabulary

Full glossary: `agentarch/std/standards/00-index.md`. Five terms carry the invariants:

- **Effect** — what a tool does to the world: `read`, `write`, `irreversible`, `money`,
  `communication`. Drives approval and guardrails.
- **Autonomy** — how far the agent may go unattended: `L0_suggest` … `L4_autonomous`.
- **Guardrail** — a check at user **input**, model **output**, or tool **action**. Not a
  prompt instruction.
- **Fail mode** — when a guardrail cannot decide: `fail_closed` (block), `fail_warn` (record),
  `fail_open` (allow).
- **Untrusted content** — anything not authored by your code. Always data, never instruction.
