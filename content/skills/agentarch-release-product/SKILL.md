---
name: agentarch-release-product
description: Take reviewed software to production safely — write the release plan before verification, record the deployment a person or CI performed, and record the post-deploy check, without deploying anything yourself.
metadata:
  version: "1.0.0"
---

# Release a verified product

Write `agentarch/project/release.md` before final verification, so the reviewer reads how the
change ships and how it is undone. It needs five filled sections: `## Environment`,
`## Owner` (a named person), `## Rollback` (the exact steps, including data or migrations),
`## Post-deploy verification` (what to run or look at, and what counts as healthy) and
`## Signals` (what to watch, and for how long). A plan added after review changes the source
and sends the work back to verification.

Do not deploy, merge or change production yourself unless the user explicitly authorizes that
specific action through the project's own release mechanism. Releasing is a separate decision
from preparing a PR.

After a person or CI deploys, check out the deployed commit and run
`agentarch workflow record-release --url <deployment run> --head <commit> --environment <id>`.
It is refused unless that commit carries exactly the verified and reviewed content; a squash or
rebase merge of the same tree is accepted, anything else needs a new verification and review.

Then perform or ask for the post-deploy verification from the plan and record what was
observed with `agentarch workflow record-production-check --outcome passed|failed --attester
<who> --summary <what was seen> --url <dashboard, log or report>`. This is an attestation:
AgenteARQ has no network and does not observe production, so never record a check that was not
actually performed. If it failed, say so, and follow the rollback in the plan.

Stop at `production_verified`, or at the precise unmet prerequisite. Any later change to the
source starts a new cycle at verification.
