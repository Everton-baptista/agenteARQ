---
name: agentarch-upgrade-review
description: >-
  Review what a newer agentarch release changes for this project before upgrading. Use when the
  user asks to upgrade agentarch, asks what a new version breaks, or when `agentarch upgrade`
  reported locally edited files.
---

# Reviewing an upgrade

An upgrade replaces `agentarch/std/` wholesale. Nothing under `agentarch/project/` is touched,
and `agentarch.yaml` is left alone — but controls can enter warn mode, and a content major can
raise a severity.

## 1. Look before writing

```bash
agentarch upgrade --dry-run
```

This reports vendored files edited locally, comparing against `LOCK.json` — the record of what
was installed, not against the new payload. Those edits will be lost.

**If files were edited**, ask what the edit was for. There are four supported places to
customise, and none of them is `std/`:

| Need | Where it belongs |
|---|---|
| an exception, with a deadline | `agentarch waive` |
| a rule of your own | a pack under `project/`, or your own pack |
| project-specific assistant guidance | an `agentarch:custom` region in the generated file |
| your artifacts | `agentarch/project/` |

If several projects need the same edit under `std/`, that is evidence the standard is wrong.
Say so, and point at the RFC process rather than working around it.

## 2. Read what changes

```bash
python3 agentarch/std/skills/agentarch-upgrade-review/scripts/diff_controls.py
```

It compares the installed control catalogue with the one the new binary carries, and reports:
controls added, severities raised, grace periods that have now elapsed, and controls removed.

Focus on what will start failing. A control that was in warn mode and is now enforced is the
common surprise, and it is not a regression — it is the grace period ending as designed.

## 3. Upgrade and re-run the gate

```bash
agentarch upgrade
agentarch check --profile standard
```

New failures are expected. For each: `agentarch explain <control.id>`, then either fix it or
take the debt deliberately with a dated, owned waiver.

## 4. Report

Tell the user what changed, what now fails, and what it would take to fix each. If nothing
changed for this project, say that — most upgrades should be uneventful, and saying so plainly
is more useful than manufacturing significance.
