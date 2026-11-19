# Checklist: review an upgrade

Mirrors `agentarch-upgrade-review`.

## 1. Look before writing

```bash
agentarch upgrade --dry-run
```

- [ ] Locally edited vendored files reviewed. Those changes will be lost

If files were edited, ask what for. None of the four supported places to customise is `std/`:

| Need | Where |
|---|---|
| an exception with a deadline | `agentarch waive` |
| a rule of your own | your own pack under `project/` |
| assistant guidance for this project | an `agentarch:custom` region |
| your artifacts | `agentarch/project/` |

- [ ] If several projects need the same edit under `std/`, raised as an RFC instead of worked
      around. That is evidence the standard is wrong

## 2. Read what changes

```bash
python3 agentarch/std/skills/agentarch-upgrade-review/scripts/diff_controls.py
```

- [ ] Controls added, severities raised, grace periods elapsed, controls removed
- [ ] Understood that a control leaving warn mode is the grace period ending as designed, not a
      regression

## 3. Upgrade

```bash
agentarch upgrade
agentarch check --profile standard
```

- [ ] Each new failure: `agentarch explain <control.id>`, then fixed or waived with an owner and
      a date

## 4. Report

- [ ] What changed, what fails, what each fix costs
- [ ] If nothing changed for this project, said plainly. Most upgrades should be uneventful
