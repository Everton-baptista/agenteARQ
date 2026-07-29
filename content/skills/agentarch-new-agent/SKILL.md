---
name: agentarch-new-agent
description: >-
  Create a new AI agent under the agentarch standard. Use when the user asks to build, add or
  scaffold an agent; when they describe something an agent should do and no manifest exists yet;
  or when they mention agent.yaml, a system prompt, tool specs or the agentarch gate in the
  context of starting something new. Not for reviewing an agent that already exists — that is
  agentarch-review-agent.
---

# Creating an agent

Your job is to end with `agentarch validate` and `agentarch check` both passing, and with a
manifest that says what the agent actually does.

**Drive the CLI. Do not hand-write what a command produces**, and do not invent field names —
the schemas are in `agentarch/std/schemas/` and `agentarch explain <control.id>` answers why any
rule exists.

## 1. Ask whether a blueprint already fits

```bash
agentarch blueprint list
```

If one matches what the user described, install it and adapt. Starting from something that
already passes the gate is faster than assembling one and discovering later what was missing.

```bash
agentarch blueprint add <id> --yes
```

Then skip to step 4.

## 2. Otherwise scaffold

```bash
agentarch new agent <id>
```

Lowercase kebab-case, English, describing the job rather than the technology:
`invoice-triage`, not `gpt-helper`.

## 3. Fill in the manifest, in this order

The order matters. Each answer constrains the next, and doing them out of order produces a
manifest that describes a system nobody decided on.

**`owner.accountable`** — a named person. Ask who it is; do not guess and do not put a team
name there. If the user cannot name someone, say plainly that the agent is not ready for the
stage it claims.

**`out_of_scope`** — at least one entry, and this is the field to spend time on. Ask: *"what
are the three things you would be most alarmed to find it had done?"* Write those. A capable
model asked to do something adjacent to its purpose will usually try, and produce something
plausible.

**`autonomy.level`** — a property of the deployment, not of the model. Use
`references/autonomy-levels.md`. When unsure, choose lower: raising it later is a deliberate
decision with a revalidation trigger, lowering it after an incident is not.

**`stop_conditions`** — observable states. "Answer delivered with a citation" is observable.
"The task is complete" is the agent's own judgement, which is the thing under test.

**`model`** — pinned, with an immutable identifier. Never an alias ending in `latest`.

**tools** — for each capability, `agentarch new tool <id> --effect <effect>`. Classify the
effect **before** writing the implementation; it determines approval, guardrails and blast
radius. `communication` is separate from `write` on purpose: undoing a write is a database
operation, undoing a sent message is an apology.

**`guardrails`** — all three keys present. An empty list records a decision; a missing key
records an oversight.

## 4. Write the system prompt

`prompts/system.v1.md`, in layers: role → scope and non-scope → tool policy → refusal policy →
output format → untrusted block.

Two things to get right:

- **Mirror `out_of_scope` into the refusal section.** The model does not infer exclusions from a
  positive description; it infers that adjacent things are probably fine.
- **The untrusted block goes last**, with an explicit statement that instructions appearing
  inside it are evidence of tampering. Tags with no explanation are decoration.

After editing the prompt, the hash in the manifest is stale. `agentarch validate` will say so
(AA-REF-004) — update `sha256` and bump `version`.

## 5. Verify, and do not stop before it passes

```bash
agentarch validate
agentarch check --profile standard
```

When a control fails, run `agentarch explain <control.id>` and fix the cause. **Never widen a
permission to make a failure go away** — the failure is information about the task being
wrong-shaped. If the user genuinely needs to ship with a gap, use `agentarch waive` with an
owner and a date, and say out loud that it is debt.

## 6. Report what you did

Name the agent, its autonomy level, what it refuses, and any waiver you recorded. If anything is
still `TODO`, say which fields and why.
