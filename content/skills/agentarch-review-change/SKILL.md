---
name: agentarch-review-change
description: Independently review a software change against its requirements and evidence, prioritizing reproducible correctness, security, compatibility, and maintainability findings.
metadata:
  version: "1.1.0"
---

# Review independently

Read the requirements, actual diff, surrounding implementation, and executed test evidence.
Evaluate the change as a reviewer distinct from its author. If you wrote the change, call
your inspection a self-check and obtain a separate review where the workflow requires it.

When no second person is available, run the review in a fresh-context, read-only subagent
(`agentarch-reviewer` on hosts that support subagents) given only the requirements, scenarios,
diff and evidence, never the author's reasoning. Record it with `--kind independent_agent`, and
say in the summary that an agent review is weaker evidence than a human one. Never invent a
human reviewer's name.

Prioritize concrete defects with a trigger, consequence, and precise location. Check trust
boundaries, failure paths, public compatibility, data migration, and missing behavioral
coverage in proportion to the change. Avoid stylistic findings already covered by tooling.

Verify suspicious behavior before reporting it where practical. Label uncertainty and do
not manufacture findings to fill a quota. An LLM review is evidence to assess, not the sole
release gate; deterministic required checks must also pass.

Return findings by severity and the reviewed revision, or state that no actionable defect
was found with the limits of the review. Then give the owner five plain-language lines: what
changed, what was checked, what was found, what risk remains, and what they need to decide. Never mark your own review as independent through
a progress checkpoint. Fixes invalidate affected review evidence and require reinspection.
