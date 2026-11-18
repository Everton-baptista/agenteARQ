---
name: agentarch-verify-product
description: Verify implemented acceptance criteria using executed checks, contract tests, and applicable UI or accessibility checks, and record revision-bound evidence before delivery.
metadata:
  version: "1.0.0"
---

# Verify the delivered behavior

Map each acceptance criterion to an executed check or an explicit manual validation. Read
the project's configured commands; do not infer success from generated code or prose.
Record the revision, command, exit status, and evidence location for each required check.

Use the narrowest meaningful test layer: unit behavior, integration boundaries, public
contracts, and essential end-to-end journeys. For UI changes, inspect real rendering,
keyboard operation, focus, and relevant accessibility assertions. Automation alone does
not establish complete accessibility or security.

Run relevant dependency, secret, and static checks that the project requires. Verify
migrations and recovery when present. A missing executable, timeout, or unavailable service
means the check did not pass; retain that distinction in the result.

AgenteARQ's `verify_project` records its own checks; it does not certify every property of
the product. Report any coverage gap. Invalidate stale evidence after implementation changes.
Stop with a clear pass/fail/incomplete outcome tied to the tested revision.
