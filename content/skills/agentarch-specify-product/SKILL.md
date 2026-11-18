---
name: agentarch-specify-product
description: Turn an accepted product brief or change request into traceable requirements, acceptance criteria, and bounded implementation tasks for any software stack.
metadata:
  version: "1.0.0"
---

# Specify a software change

Read the accepted brief and existing contracts. Give each requirement a stable identifier
and observable acceptance criteria, including the important failure and permission cases.
Do not invent business rules to fill consequential gaps; surface those decisions.

Separate externally observable behavior from implementation choices. Describe interfaces,
data ownership, compatibility, and migration only to the depth needed for this change.
Keep the specification current when an accepted decision changes.

Split the work into bounded vertical tasks. Each task names the requirement it satisfies,
prerequisites, affected boundary, and validation that will demonstrate completion. Allow
parallel work only when write ownership and dependencies are clear.

Use the project's existing artifact format and AgenteARQ workflow when present. A task is
not complete because a checkbox changed: its acceptance criteria need independent evidence.
Stop with an implementable task sequence or a precise unresolved decision.
