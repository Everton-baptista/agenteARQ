# Checklist: bootstrap evals

Mirrors `agentarch-eval-bootstrap`. This is the only evidence that separates a declared control
from a working one, which is why conformance L3 rests on it.

## 1. Thresholds first

- [ ] For each metric, answered **before running anything**: *below what number would you not
      ship this?*

Written afterwards, a threshold describes whatever the system scored, and the gate can then only
ever detect a regression.

## 2. Metrics from the closed vocabulary

From `standards/11-evaluation.md`. Different names for the same quantity make the numbers
meaningless.

- [ ] Retrieval-grounded: `groundedness`, `citation_accuracy`, `context_recall`
- [ ] Generative: `refusal_correctness`, `pii_leakage`, `jailbreak_success_rate`
- [ ] Agentic: `task_success`, `tool_selection_accuracy`, **`unauthorized_action_blocked`**

The last one is most often missing and matters most: it measures the action guardrail.

## 3. Two datasets

```bash
python3 agentarch/std/skills/agentarch-eval-bootstrap/scripts/seed_dataset.py <agent-id>
```

- [ ] Every `TODO` replaced. A case with a placeholder input measures nothing
- [ ] One red-team case per `out_of_scope` entry
- [ ] **Golden set is paraphrased or synthetic, never real tickets.** Real ones end up in git, in
      CI, on laptops and in any fork
- [ ] Both `sha256` values recorded in the plan. Without them, a quietly edited dataset makes a
      regression look like an improvement

## 4. If there is a judge

- [ ] Judge model versioned, prompt hashed
- [ ] Agreement with human labels measured and recorded
- [ ] `paired_with` names a deterministic metric
- [ ] **Not in `gate.block_on` alone.** A drifting judge produces irreproducible failures, and
      the remedy people reach for is disabling the whole gate

## 5. Wire it up

- [ ] `evaluation.plan_ref`, `last_result_ref`, `max_result_age_days` set
- [ ] `agentarch check` passes
- [ ] Understood that extending `max_result_age_days` because it blocked a release is how a
      freshness control stops meaning anything
