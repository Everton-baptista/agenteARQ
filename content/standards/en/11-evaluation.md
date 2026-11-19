# 11. Evaluation

Purpose: turning declarations into evidence.
Version: 0.1 · Status: draft · Scope: `evals/plan.yaml`, `evals/results/*`, the release gate.

Every other standard in this catalogue can be satisfied by writing something down. This one
cannot. An eval result is the only artifact that distinguishes *we declared a guardrail* from
*the guardrail works*, which is why conformance level L3 rests entirely on it.

---

## 1. Rules

### control.ai.eval.plan_exists

**Intent.** Someone decided what "working" means before shipping.
**Severity** `major`

`evals/plan.yaml` names datasets, metrics, thresholds and what blocks a release. Written before
the numbers exist, so the thresholds are a judgement about acceptable behaviour rather than a
description of whatever the system happened to score.

### control.ai.eval.dataset_versioned

**Intent.** Two results are comparable.
**Severity** `blocker`

Every dataset carries a `sha256` and a case count. Without it, a quietly edited dataset makes a
regression look like an improvement, and nobody can tell which run the numbers came from.

### control.ai.eval.gate_thresholds

**Intent.** A number nothing acts on is a metric, not a control.
**Severity** `blocker`

`gate.block_on` names the metrics that block. A plan that measures everything and blocks on
nothing is a dashboard.

### control.ai.eval.result_fresh

**Intent.** The result describes the system you are shipping.
**Severity** `blocker`

`completed_at` within `max_result_age_days`. A missing date is not fresh — `age_days` on a
missing value yields null, and comparisons against null are false, so the control fails rather
than passing because nothing was recorded.

The result also records its `subject`: model id, prompt version and hash, corpus version. When
any of those differ from the agent as it stands, the result describes a different system. That
is what the revalidation triggers in `14-lifecycle.md` exist to catch.

### control.ai.eval.redteam_executed

**Intent.** Somebody attacked it on purpose.
**Severity** `blocker` for public-facing agents, `major` otherwise

A red-team dataset, seeded from the attack catalogue in `09-security.md`, with the count of
successful attacks recorded. Zero successes on ten cases is not a result; it is a small sample.

### control.ai.eval.judge_not_sole_blocker

**Intent.** The gate stays reproducible.
**Severity** `blocker`

An LLM-as-judge metric never blocks alone. It is paired with a deterministic metric, its model
and prompt are versioned and hashed, and its agreement with human labels is measured.

A judge whose behaviour drifts between releases produces failures nobody can reproduce. The
gate then loses credibility as a whole — and the team's remedy is to disable it, which takes the
deterministic checks down too. The rule protects the deterministic checks, not the judge.

---

## 2. Metrics by system type

A closed vocabulary, so numbers mean the same thing across projects. Different names for the
same quantity make a standard's numbers meaningless.

| System type | Measure |
|---|---|
| **Retrieval-grounded** | `groundedness`, `faithfulness`, `context_recall`, `context_precision`, `citation_accuracy`, `answer_relevance` |
| **Generative** | `hallucination_rate`, `pii_leakage`, `jailbreak_success_rate`, `toxicity`, `refusal_correctness`, `format_conformance` |
| **Agentic** | `task_success`, `tool_selection_accuracy`, `unauthorized_action_blocked`, `loop_rate`, `human_intervention_rate`, `steps_per_task`, `cost_per_task` |

`unauthorized_action_blocked` is the one most often missing and the one that matters most for an
agent: it measures the action guardrail, which is the last checkpoint before something
irreversible happens.

---

## 3. What a judge is for

Deterministic checks answer questions with a right answer: did it cite a source, did it leak an
identifier, did it emit valid JSON, did the guardrail block. Use them wherever they reach.

A judge is for the residue — whether an answer is *useful* — which no deterministic check
reaches. That makes it worth having and unsuitable as a gate:

| Requirement | Why |
|---|---|
| Versioned model and hashed prompt | it is a model with a prompt, so it is a dependency |
| Calibrated against human labels, with agreement recorded | an uncalibrated judge produces a number, not a measurement |
| Paired with a deterministic metric | so a drifting judge cannot take the gate down |
| Never the sole blocker | see above |

---

## 4. Do / don't

| Do | Don't |
|---|---|
| Write thresholds before you have numbers | Set the threshold to just below what it scored |
| Hash the dataset | Edit cases in place and compare across runs |
| Record what was evaluated — model, prompt hash, corpus | Record only the scores |
| Grow the red-team set from real incidents | Freeze it at the initial ten cases |
| Let a stale result fail the gate | Extend `max_result_age_days` to make it pass |
| Report the judge as advisory | Block on it because it looks like a number |

---

## 5. Affected artifacts and fields

`evals/plan.yaml`: `datasets[].sha256`, `metrics[].{kind,threshold,direction}`,
`metrics[].judge.*`, `gate.block_on`, `gate.max_result_age_days`.
`evals/results/*`: `completed_at`, `subject.*`, `metrics[].passed`, `deterministic_metrics`,
`redteam.*`.
`agent.yaml`: `evaluation.{plan_ref,last_result_ref,max_result_age_days}`.

---

## 6. Observed anti-patterns

**Thresholds fitted to the score.** The system scored 0.83 and the threshold became 0.80. The
gate can now never fail without a regression, which is the only thing it will ever detect.

**The dataset that drifts.** Cases fixed in place as bugs are found. Every run is measured
against a slightly different exam.

**Ten red-team cases, zero successes.** Reported as "no vulnerabilities found".

**The judge that became the gate.** It was the easiest metric to add. Two releases later it
scores differently on unchanged output, nobody can reproduce the failure, and the gate is off.

**Freshness extended instead of re-run.** `max_result_age_days: 30` becomes `365` on the day it
first blocks a release.

**Results with no subject.** Good numbers, and no way to know which model or prompt produced
them.

---

## 7. External references

Reviewed 2026-07-28. Mappings only.

- NIST AI RMF 1.0 — *Measure*, on measurement that is tied to intended use.
- MITRE ATLAS — source material for red-team case categories.
- OWASP Top 10 for LLM Applications — the failure modes red-team datasets should cover.
