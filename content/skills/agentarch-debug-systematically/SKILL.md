---
name: agentarch-debug-systematically
description: Investigate a reproducible software failure, identify its cause, and verify a narrowly scoped fix with a regression test rather than changing code speculatively.
metadata:
  version: "1.0.0"
---

# Debug from evidence

Capture the observed failure, expected behavior, affected revision, and minimal reproducer.
Inspect existing diagnostics without collecting credentials or personal data unnecessarily.
If the failure is intermittent, state the conditions and uncertainty explicitly.

Form a falsifiable hypothesis and run the cheapest discriminating check. Follow data across
the failing boundary; do not change several plausible causes simultaneously. Distinguish
environment failures from product defects and report both honestly.

Add a regression test that fails for the demonstrated cause, apply a scoped fix, and run
the relevant suite. Verify the original symptom, nearby behavior, and any failure handling
the change touched. Do not widen privileges or silence checks to make the run pass.

Record the cause, fix, executed verification, and unresolved limits. Stop when the symptom
is verified fixed or further investigation needs unavailable evidence or a user decision.
