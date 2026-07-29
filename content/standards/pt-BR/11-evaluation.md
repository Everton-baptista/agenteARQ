---
lang: pt-BR
source: content/standards/en/11-evaluation.md
source_sha256: "64dca34d90b6c86ad00ce602c51e404420a20122f927f9fe3457b34bc206e8b4"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 11. Avaliação

Propósito: transformar declarações em evidência.
Versão: 0.1 · Status: draft · Escopo: `evals/plan.yaml`, `evals/results/*`, the release gate.

Every other standard in this catalogue can be satisfied by writing something down. This one
cannot. An eval result is the only artifact that distinguishes *we declared a guardrail* from
*the guardrail works*, which is why conformance level L3 rests entirely on it.

---

## 1. Regras

### control.ai.eval.plan_exists

**Intenção.** Someone decided what "working" means before shipping.
**Severidade** `major`

`evals/plan.yaml` names datasets, metrics, thresholds and what blocks a release. Written before
the numbers exist, so the thresholds are a judgement about acceptable behaviour rather than a
description of whatever the system happened to score.

### control.ai.eval.dataset_versioned

**Intenção.** Two results are comparable.
**Severidade** `blocker`

Every dataset carries a `sha256` and a case count. Without it, a quietly edited dataset makes a
regression look like an improvement, and nobody can tell which run the numbers came from.

### control.ai.eval.gate_thresholds

**Intenção.** A number nothing acts on is a metric, not a control.
**Severidade** `blocker`

`gate.block_on` names the metrics that block. A plan that measures everything and blocks on
nothing is a dashboard.

### control.ai.eval.result_fresh

**Intenção.** The result describes the system you are shipping.
**Severidade** `blocker`

`completed_at` within `max_result_age_days`. A missing date is not fresh — `age_days` on a
missing value yields null, and comparisons against null are false, so the control fails rather
than passing because nothing was recorded.

The result also records its `subject`: model id, prompt version and hash, corpus version. When
any of those differ from the agent as it stands, the result describes a different system. That
is what the revalidation triggers in `14-lifecycle.md` exist to catch.

### control.ai.eval.redteam_executed

**Intenção.** Somebody attacked it on purpose.
**Severidade** `blocker` for public-facing agents, `major` otherwise

A red-team dataset, seeded from the attack catalogue in `09-security.md`, with the count of
successful attacks recorded. Zero successes on ten cases is not a result; it is a small sample.

### control.ai.eval.judge_not_sole_blocker

**Intenção.** The gate stays reproducible.
**Severidade** `blocker`

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

## 4. Deve / não deve

| Deve | Não deve |
|---|---|
| Write thresholds before you have numbers | Set the threshold to just below what it scored |
| Hash the dataset | Edit cases in place and compare across runs |
| Record what was evaluated — model, prompt hash, corpus | Record only the scores |
| Grow the red-team set from real incidents | Freeze it at the initial ten cases |
| Let a stale result fail the gate | Extend `max_result_age_days` to make it pass |
| Report the judge as advisory | Block on it because it looks like a number |

---

## 5. Artefatos e campos afetados

`evals/plan.yaml`: `datasets[].sha256`, `metrics[].{kind,threshold,direction}`,
`metrics[].judge.*`, `gate.block_on`, `gate.max_result_age_days`.
`evals/results/*`: `completed_at`, `subject.*`, `metrics[].passed`, `deterministic_metrics`,
`redteam.*`.
`agent.yaml`: `evaluation.{plan_ref,last_result_ref,max_result_age_days}`.

---

## 6. Antipadrões observados

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

## 7. Referências externas

Revisado em 2026-07-28. Mappings only.

- NIST AI RMF 1.0 — *Measure*, on measurement that is tied to intended use.
- MITRE ATLAS — source material for red-team case categories.
- OWASP Top 10 for LLM Applications — the failure modes red-team datasets should cover.
