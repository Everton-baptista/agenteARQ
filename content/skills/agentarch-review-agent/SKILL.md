---
name: agentarch-review-agent
description: >-
  Audit an existing AI agent against the agentarch standard and report what is wrong, ranked.
  Use when the user asks to review, audit or assess an agent; when the gate is failing and they
  want to understand why; before a release; or after an incident. Not for creating an agent —
  that is agentarch-new-agent.
---

# Reviewing an agent

Produce a ranked, honest assessment. A review that lists forty observations of equal weight is a
review nobody acts on.

## 1. Gather everything in one pass

```bash
python3 agentarch/std/skills/agentarch-review-agent/scripts/collect_context.py <agent-id>
```

This prints the manifest, the system prompt, every tool spec, the MCP allowlist, the latest eval
result and the machine-readable gate output together. Read it before forming an opinion — twelve
separate file reads produce twelve separate partial pictures.

## 2. Run the tools first

```bash
agentarch validate
agentarch check --profile standard --format json
agentarch conformance
agentarch mcp audit
```

These are cheap and precise. **Do not restate their findings as if you found them** — say which
you confirmed, then spend your effort on what they cannot see.

## 3. Then look for what no control can catch

This is where a review earns its keep. Use `references/review-rubric.md`.

**Does the manifest describe the agent that exists?** Read the prompt and the tool specs against
the code. The manifest is the contract; where they disagree, one of them is a bug, and saying
which is the reviewer's job.

**Does `out_of_scope` appear in the prompt?** A refusal declared in the manifest and absent from
the prompt is a refusal the model never learned.

**Where does identity come from?** Look at every tool signature. A `customer_id`, `user_id` or
`account_id` parameter means the model chooses whose data is touched — the most common route
from injection to breach, and it usually reads as a reasonable API design.

**Does the action guardrail consult the model?** A check that shares the model's context shares
its compromise. Look for an "is this safe?" call in the tool path.

**Is the retrieved content isolated, structurally?** Grep for string concatenation into the
system prompt. `context.rag.untrusted: true` is a declaration; the code is the fact.

**Are the eval thresholds fitted to the score?** A threshold of 0.80 next to a score of 0.83
means the gate can only ever detect a regression. Ask when the thresholds were written.

**Is the red-team set real?** Ten cases and zero successes is a small sample reported as a
result.

## 4. Rank by reachable damage

Order findings by what an attacker or an accident could actually reach, not by how many controls
they touch:

1. **Reachable now** — an unapproved irreversible tool, identity from model output, an
   unrestricted egress, a secret in the repository.
2. **Reachable after one more mistake** — a missing action guardrail where tools are narrow, a
   stale eval, an unpinned model.
3. **Weakens assurance** — undocumented decisions, missing threat model, expired review.

## 5. Report

For each finding: what is wrong, what it lets happen, and the smallest change that fixes it.
Cite the control id where one exists so the user can run `agentarch explain`.

Say what is **good** too, specifically. A review that only lists problems gets read once.

If nothing serious is wrong, say that plainly rather than manufacturing findings to look
thorough.
