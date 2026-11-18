---
name: agentarch-review-change
description: Independently review a software change against its requirements and evidence, prioritizing reproducible correctness, security, compatibility, and maintainability findings.
metadata:
  version: "1.0.0"
---

# Review independently

Read the requirements, actual diff, surrounding implementation, and executed test evidence.
Evaluate the change as a reviewer distinct from its author. If you wrote the change, call
your inspection a self-check and obtain a separate review where the workflow requires it.

Prioritize concrete defects with a trigger, consequence, and precise location. Check trust
boundaries, failure paths, public compatibility, data migration, and missing behavioral
coverage in proportion to the change. Avoid stylistic findings already covered by tooling.

Verify suspicious behavior before reporting it where practical. Label uncertainty and do
not manufacture findings to fill a quota. An LLM review is evidence to assess, not the sole
release gate; deterministic required checks must also pass.

Return findings by severity and the reviewed revision, or state that no actionable defect
was found with the limits of the review. Never mark your own review as independent through
a progress checkpoint. Fixes invalidate affected review evidence and require reinspection.
