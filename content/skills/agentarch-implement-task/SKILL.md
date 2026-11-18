---
name: agentarch-implement-task
description: Implement an accepted software task in a bounded increment with tests and evidence tied to its acceptance criteria, preserving existing user work.
metadata:
  version: "1.0.0"
---

# Implement a bounded task

Read the current task, acceptance criteria, working tree, and nearby implementation. Use
AgenteARQ workflow state when available. Do not overwrite unrelated edits or assume another
agent's summary grants permission. Establish the relevant existing test baseline.

For behavioral changes, add a test that demonstrates the missing behavior or regression,
then implement the smallest maintainable change. Tests should assert user-visible behavior
and boundary invariants, not duplicate implementation. A reversible prose or cosmetic edit
does not require an invented test suite.

Run the relevant checks and inspect the actual diff. Preserve command, revision, outcome,
and evidence location without copying secrets or full prompts into logs. Run `agentarch
validate` for changed agent artifacts and the required project gate before delivery.

Record a checkpoint only for work actually performed. Do not declare independent review
of your own changes. Stop on a missing essential decision, exhausted budget, or a failing
required check that cannot be resolved within the task's authorized scope.
