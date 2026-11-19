---
name: agentarch-eval-bootstrap
description: >-
  Create an evaluation plan and a seed dataset for an agent. Use when the user asks how to test
  or evaluate an agent, when eval controls are failing the gate, when conformance L3 is blocked
  on evidence, or when they ask what to measure.
---

# Bootstrapping evals

Every other standard can be satisfied by writing something down. This one cannot, which is why
conformance L3 rests entirely on it.

## 1. Write the thresholds before you have numbers

This is the whole discipline. Thresholds written first are a judgement about acceptable
behaviour; written afterwards they describe whatever the system happened to score, and the gate
can then only ever detect a regression.

Ask the user: *"below what number would you not ship this?"* — for each metric, before running
anything.

## 2. Pick metrics from the closed vocabulary

Different names for the same quantity make a standard's numbers meaningless. Use the list in
`agentarch/std/standards/11-evaluation.md`.

| System type | Start with |
|---|---|
| retrieval-grounded | `groundedness`, `citation_accuracy`, `context_recall` |
| generative | `refusal_correctness`, `pii_leakage`, `jailbreak_success_rate`, `format_conformance` |
| agentic | `task_success`, `tool_selection_accuracy`, `unauthorized_action_blocked` |

`unauthorized_action_blocked` is the one most often missing and the one that matters most for an
agent: it measures the action guardrail, which is the last checkpoint before something
irreversible happens.

## 3. Build two datasets

```bash
python3 agentarch/std/skills/agentarch-eval-bootstrap/scripts/seed_dataset.py <agent-id>
```

It derives a golden set from `purpose` and a red-team set from `out_of_scope` plus the attack
categories in `09-security.md`.

**The seed is a seed.** Twenty generated cases are a starting point, not a test suite. Tell the
user plainly that the set has to grow from real traffic and real incidents, and that ten
red-team cases with zero successes is a small sample reported as a result.

**Never build the golden set from real tickets.** It ends up in git, in CI, on laptops and in
any fork. Paraphrase or synthesise.

## 4. Constrain the judge

If the user wants an LLM judge — and for "is this answer useful" there is no alternative — then:

- version the judge model and hash its prompt
- record its agreement with human labels
- set `paired_with` to a deterministic metric
- **never** put it in `gate.block_on` alone

A judge that drifts between releases produces failures nobody can reproduce, and the team's
remedy is to disable the whole gate — taking the deterministic checks with it. The rule protects
those, not the judge.

## 5. Wire it up

Set `evaluation.plan_ref`, `last_result_ref` and `max_result_age_days` in the manifest. Thirty
days is a reasonable default; whatever you choose, extending it later because it blocked a
release is how a freshness control stops meaning anything.

```bash
agentarch validate
agentarch check --profile standard
```
