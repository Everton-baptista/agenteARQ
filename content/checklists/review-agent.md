# Checklist: review an agent

Mirrors `agentarch-review-agent`. Produce a **ranked** assessment — a list of forty equal-weight
observations is one nobody acts on.

## 1. Run the tools first

```bash
agentarch validate
agentarch check --profile standard --format json
agentarch conformance
agentarch mcp audit
```

- [ ] Findings noted. **Do not restate these as your own** — say which you confirmed, then spend
      your effort on what tools cannot see.

## 2. Read everything at once

```bash
python3 agentarch/std/skills/agentarch-review-agent/scripts/collect_context.py <agent-id>
```

Twelve separate file reads produce twelve partial pictures, and an opinion formed before seeing
the tool that contradicts it.

## 3. What no control catches

- [ ] **Does the manifest describe the agent that exists?** Read the prompt and tool specs
      against the code. Where they disagree, one is a bug — say which.
- [ ] **Does `out_of_scope` appear in the prompt?** A refusal declared and not taught is a
      refusal the model never learned.
- [ ] **Where does identity come from?** Check every tool signature for a `customer_id`,
      `user_id` or `account_id` parameter. That means the model chooses whose data is touched —
      the most common route from injection to breach, and it reads as reasonable API design.
- [ ] **Does the action guardrail call the model?** A check sharing the model's context shares
      its compromise.
- [ ] **Is retrieved content isolated in the code?** `untrusted: true` is a declaration; grep
      for concatenation into the system prompt for the fact.
- [ ] **Were the eval thresholds written before the scores?** 0.80 next to 0.83 means the gate
      can only detect a regression.
- [ ] **Is the red-team set real?** Ten cases and zero successes is a small sample reported as a
      result.

## 4. Rank by reachable damage

1. **Reachable now** — unapproved irreversible tool, identity from model output, unrestricted
   egress, a secret in the repository
2. **Reachable after one more mistake** — missing action guardrail, stale eval, unpinned model
3. **Weakens assurance** — undocumented decisions, no threat model, expired review

## 5. Report

- [ ] Each finding: what is wrong, what it lets happen, the smallest fix
- [ ] Control id cited where one exists
- [ ] What is **good**, specifically. A review that only lists problems gets read once
- [ ] If nothing serious is wrong, say so rather than manufacturing findings
