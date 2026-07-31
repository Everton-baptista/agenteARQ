# 03. Pack resolution (normative)

Resolution turns a profile and a manifest into a set of controls, each with one severity. It
must be deterministic and it must be explainable — nobody should have to read an implementation
to find out why they are blocked.

## 1. Selecting packs

The selected set is the **union** of:

1. the packs named by the active profile, and
2. the packs named in `agent.policy.packs`.

Union, never override. An agent may add obligations to its project's profile; it MUST NOT be
able to remove them. An implementation MUST NOT provide a manifest-level way to opt out of a
profile pack — the supported way to accept a specific failure is a waiver, which has an owner
and an expiry.

An implementation MUST report a named pack it cannot find, and MUST continue with the rest
rather than failing. A missing pack is a configuration problem; refusing to evaluate anything
because of it hides the failures that were already there.

## 2. Applicability of a pack

A pack applies when every condition in `applies_when` holds:

| Condition | Matches when |
|---|---|
| `system_type` | the manifest's `system_type` is in the list |
| `stage` | the manifest's `stage` is in the list |
| `audience` | `users.audience` is in the list |
| `processes_personal_data` | `privacy.processes_personal_data` equals the declared boolean |
| `jurisdictions` | **at least one** of the agent's `jurisdictions` is in the list |

Jurisdiction matching is intersection, not subset. An agent operating in the EU and Brazil picks
up both regimes; an agent operating only in the US picks up neither. This is what lets one core
serve teams in different countries, and an implementation that imposed every regional pack on
everyone would make the standard un-adoptable outside one jurisdiction.

An absent `applies_when` means the pack always applies.

## 3. Merging severities

When two applicable packs require the same control:

1. The **highest** severity wins, ordered `blocker` > `major` > `minor` > `warn`.
2. A requirement inside its grace period (`enforced_from` above the pack's own version) is
   evaluated at `warn` **before** step 1 is applied.
3. A control required by a `binding_law` pack MUST NOT resolve below the severity that pack
   declared, whatever another pack says.

Resolving downward would let an organisation pack quietly weaken a security pack it imported,
which is a supply-chain problem wearing a configuration hat.

## 4. Explainability

An implementation MUST provide a way to show, for each resolved control: the severity, the pack
and version that imposed it, and any requirement it superseded.

This is a MUST rather than a SHOULD. A gate that blocks without being able to say which document
imposed the rule is a gate people route around, and the routing is invisible.

## 5. Evaluation order and independence

Controls MUST be evaluated independently. An implementation MUST NOT let one control's outcome
change another's, MUST NOT stop at the first failure, and MUST report every applicable control.

Order MUST NOT affect results. An implementation MAY evaluate in parallel.

## 6. Waivers

A waiver suppresses a **failure**, not a control. An implementation MUST:

- require `control`, `owner`, `reason` and `until`
- reject an `until` more than 90 days in the future
- treat an expired waiver as a waiver problem (exit 5), **not** as an absent one

The last point is the mechanism. An expired waiver that silently stopped applying would let the
failure reappear as a fresh blocker with no trace of the decision that deferred it, and the
person who took the decision would never hear about it.

A waiver MAY name an agent. Without one it applies to every agent in the project, and an
implementation SHOULD say so when reporting it.

## 7. Baseline

A baseline suppresses failures that existed when a project adopted the standard.

An implementation that supports it MUST:

- report a baselined failure as neither passed nor blocking, and MUST count it in any maturity
  score
- refuse to cover a failure whose severity is **higher** than the one recorded
- refuse to cover a failure on an agent or control not in the baseline
- never add entries when updating; only remove ones that are now passing

Those four rules are what make it a ratchet. Without them it is an amnesty, and the debt
disappears rather than being deferred.
