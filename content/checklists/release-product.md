# Release a product

- [ ] Write `release.md` (environment, owner, rollback, post-deploy verification, signals) before final verification.
- [ ] Add backups and restore (with the date of a rehearsed restore), monitoring by name, and free-tier limits acknowledged by the owner.
- [ ] Test the rollback before the first release.
- [ ] Deploy only through an authorized mechanism; preparing a PR does not authorize a release.
- [ ] Record the deployment with `record-release` from the exact deployed commit.
- [ ] Perform the post-deploy verification and record what was actually observed, passed or failed.
- [ ] On a failed check, follow the rollback in the plan; any source change starts a new cycle.

Procedure: [agentarch-release-product](../skills/agentarch-release-product/SKILL.md).
