# 01. Agent contract

Purpose: what an agent must declare about itself before it is built.
Version: 0.1 · Status: draft · Scope: `agent.yaml`, one per agent.

An agent is not defined by its prompt. It is defined by what it is allowed to attempt, what it
must refuse, how far it may go alone, and who answers when it is wrong. Those four things are
this standard.

---

## 1. Rules

### control.ai.agent.owner_defined

**Intent.** Someone answers for this agent's behaviour.
**Severity** `blocker` · **Fail mode** n/a (structural)

`owner.accountable` names a person. A team alias, a queue, a rotation or a mailing list does
not satisfy this — those are how you *reach* the owner, which is what `owner.contact` is for.

**How to verify.** `owner.accountable` is present and non-empty.
**How to fix.** Name the individual who would be paged. If nobody would be, the agent is not
ready for the stage it claims.

### control.ai.agent.scope_declared

**Intent.** The agent's boundary is written down before it is discovered in production.
**Severity** `blocker`

`purpose` states what it does in one sentence. `out_of_scope` lists at least one thing it must
refuse.

The asymmetry is deliberate. A capable model asked to do something adjacent to its purpose will
usually try, and produce something plausible. The only reliable defence is to have decided the
boundary in advance, in writing, where it can be reviewed, tested and put into the system
prompt.

**How to verify.** `out_of_scope` has ≥ 1 entry.
**How to fix.** Write the three things you would be most alarmed to find it had done, then
mirror them into the refusal section of the system prompt.

### control.ai.agent.autonomy_declared

**Intent.** How much rope the agent has is a decision, not an emergent property.
**Severity** `blocker`

| Level | The agent may |
|---|---|
| `L0_suggest` | produce output; a human performs every action |
| `L1_act_with_approval` | act, with a human approving each action |
| `L2_act_reversible` | act alone where the action can be undone |
| `L3_act_irreversible_bounded` | act alone within declared numeric or scope limits |
| `L4_autonomous` | act alone, bounded only by budget and stop conditions |

Autonomy is not a property of the model. It is a property of the *deployment*, and the same
prompt at L1 and L4 is two different systems with two different threat models.

Raising this level is a revalidation trigger (`autonomy_raised`): prior evals and the threat
model were performed against a different system.

### control.ai.agent.stop_conditions

**Intent.** The loop terminates.
**Severity** `blocker`

`max_steps`, `max_tool_calls` and at least one `stop_conditions` entry are all required.

An agent without a termination argument does not have a bug that shows up occasionally; it has
an unbounded cost and an unbounded blast radius that happen to have been small so far. The
common failure is not an infinite loop — it is a loop of eleven steps that costs forty times the
expected amount and calls a write tool nine times.

**How to fix.** Write the conditions as observable states ("answer delivered with a citation",
"escalated to a human"), not intentions ("the task is done").

### control.ai.agent.budget_bounded

**Intent.** Cost and latency have a declared ceiling.
**Severity** `major`

`autonomy.budget` declares `usd_per_run`, `tokens_per_run` and `latency_p95_ms`. These are the
numbers an SLO and a cost alert are built from, and the numbers a denial-of-wallet attack has to
exceed to be noticed.

### control.ai.agent.secrets_by_reference

**Intent.** No secret value ever lands in a repository.
**Severity** `blocker`

Manifests, tool specs, prompts, logs and spans reference secrets by **name**. The value is
resolved at runtime from a secret manager.

**How to verify.** `permissions.secrets` entries match `^[A-Z][A-Z0-9_]*$` and no artifact
contains a high-entropy literal.

---

## 2. Do / don't

| Do | Don't |
|---|---|
| Name a person as accountable | Put a team alias in `accountable` |
| Write `out_of_scope` before the prompt | Discover the boundary from an incident |
| Set autonomy from the deployment's risk | Assume a better model justifies more autonomy |
| Express stop conditions as observable states | Write "when the task is complete" |
| Reference secrets by name | Inline a token "just for local testing" |

---

## 3. Affected artifacts and fields

`agent.yaml`: `owner.accountable`, `owner.contact`, `purpose`, `out_of_scope`,
`autonomy.level`, `autonomy.max_steps`, `autonomy.max_tool_calls`,
`autonomy.stop_conditions`, `autonomy.budget`, `stage`, `system_type`.

---

## 4. Expected evidence

| Control | Evidence |
|---|---|
| `owner_defined`, `scope_declared`, `autonomy_declared`, `stop_conditions` | manifest field |
| `budget_bounded` | manifest field, plus an observability metric once `12-observability` is adopted |
| `secrets_by_reference` | manifest field, plus a secret-scanning result over the repository |

---

## 5. Observed anti-patterns

**The team is the owner.** `accountable: platform-team`. When the agent misbehaves at 3am, the
question "who decides whether to disable it" has no answer, and the default answer is "nobody,
until morning".

**Out of scope inferred from the prompt.** The prompt says "you help with orders", and everyone
assumes refunds are excluded. The model does not infer exclusions from a positive description —
it infers that refunds are adjacent to orders and therefore probably fine.

**Autonomy raised quietly.** A tool moves from `read` to `write` and the autonomy level stays at
L2 because nobody re-read the manifest. This is why `tool_effect_raised` and `autonomy_raised`
are separate revalidation triggers.

**Stop conditions that restate the goal.** "Stops when it has answered the question" is not
observable — the agent's own judgement of whether it has answered is the thing under test.

---

## 6. External references

Reviewed 2026-07-28. The mappings here are maintained against these sources; the standard never
reproduces their text.

- NIST AI Risk Management Framework 1.0 — *Govern* and *Map* functions, for accountability and
  intended-use declarations.
- ISO/IEC 42001:2023 — management-system requirements for defined roles and responsibilities.
- OWASP Top 10 for LLM Applications — *Excessive Agency*, which this standard addresses through
  declared autonomy plus the tool permissions in `03-tools.md`.
