---
name: agentarch-prepare-pr
description: Prepare an authorized, reviewable pull request from verified software changes, with acceptance traceability, independent review evidence, and an accurate description.
metadata:
  version: "1.0.0"
---

# Prepare a verified pull request

Inspect the final diff and required acceptance evidence at the current revision. Resolve
blocking findings and ensure any required independent review covers the final change.
Do not treat a completed agent run or green governance check as proof that product tests ran.

Describe the concrete problem and resulting behavior, relevant validation, compatibility,
and material remaining limitations. Include recovery or migration notes only when needed.
Keep unrelated edits out of the change without discarding the user's working tree.

Create or update a PR only when that external action is authorized. Use the project's
controlled publication mechanism and credentials; do not assume plugin installation grants
publication access. If publication is unavailable, provide the local reviewable result and
the precise unmet prerequisite rather than claiming a PR exists.

Stop at the requested deliverable. A PR request does not imply merge, deployment, release,
or ongoing autonomous repair. Do not present an unfinished check as a successful one.
