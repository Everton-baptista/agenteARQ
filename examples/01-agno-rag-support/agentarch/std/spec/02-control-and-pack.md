# 02. Control and pack (normative)

A control is one verifiable rule. A pack is a versioned set of controls with severities.

**A pack is data, never code.** Everything in this document follows from that: packs travel
through a registry and come from third parties, and a governance tool that executed them would
hand an execution primitive to anyone who can get a pack adopted.

## 1. Control identity

`control.ai.<type>.<name>`, where `<type>` is one of `agent`, `genai`, `rag`, `tool`, `mcp`,
`api`, `privacy`, `eval`, `obs`, `supply`, `lifecycle`, and `<name>` is lowercase snake_case.

Identifiers MUST NOT be reused. A removed control's id stays removed, so a waiver or a baseline
entry naming it can never silently attach to something else.

## 2. Check kinds

`check.kind` is a closed set. An implementation MUST reject any other value.

| Kind | Evaluated by | Requires |
|---|---|---|
| `static_manifest` | the expression language over declared artifacts | `expr` |
| `eval_threshold` | the same language, over a loaded eval result | `expr` |
| `file_exists` | testing a path derived from the agent directory | `path_expr` |
| `manual_attestation` | nobody; a human asserted it | — |

There is no kind that runs a program, and an implementation MUST NOT add one. `manual_attestation`
is the honest escape for a rule that cannot be automated; it is recorded, owned and dated, and
an implementation MUST report it as skipped rather than as passed.

## 3. Required fields

A control MUST carry `id`, `type`, `title`, `intent`, `status`, `check`, `remediation` and
`standard_ref`.

`intent` says what goes wrong without the control. `remediation` says what to do. Both are
required because a finding with neither is a finding people learn to ignore, and a catalogue of
those looks like coverage while producing none.

`standard_ref` points at the prose. An implementation SHOULD verify the correspondence in both
directions (`AA-DOC-008`): a control with no prose, and prose describing a rule no control
checks, are both defects. This is the mechanism that stops the standard becoming shelfware.

## 4. Applicability

`applies_to` narrows where a control is evaluated at all.

| Condition | Applies when |
|---|---|
| `system_type` | the manifest's `system_type` is in the list |
| `autonomy_min` | `autonomy.level` is at or above the named level |
| `stage_min` | `stage` is at or above the named stage |
| `processes_personal_data` | `privacy.processes_personal_data` equals the declared boolean |
| `declares` | **every** named manifest section is present, per `exists()` in `04` |

A control that does not apply MUST be reported as skipped and MUST count as neither a pass nor a
failure. An implementation SHOULD NOT print skipped controls by default; an inapplicable control
is noise, and noise is what makes people stop reading output.

`declares` names optional top-level sections of the manifest — the ones a control needs present
before it has anything to say. A control reading `agent.interface.caller` has no question to ask
of an agent that is not reached over HTTP, and `system_type` cannot answer this: it records what
an agent **is**, not how it is **reached**, and any system type may be put behind an interface.

The list of nameable sections is closed, and MUST NOT include a section the manifest requires: a
condition on something always present can never skip, which makes a control look conditional
while it is not.

An implementation MUST NOT accept a path, an index or an expression here. Presence of a named
section is the entire vocabulary. A free-form path would make `applies_to` a second place to
write a check — evaluated before the real one, and outside every guarantee in `04` about what a
pack cannot do.

A control MUST NOT use `declares` to narrow a rule that holds regardless of the agent's shape. A
committed credential is public in a library, a batch job and a service alike; scoping such a
control to a section would silently stop it running on the projects that needed it, which is the
same defect as noise with the sign reversed.

## 5. Pack requirements

Each entry in `requires` names a control, a severity, and optionally `enforced_from` and
`required_evidence`.

**`enforced_from`** is the pack version from which the declared severity applies. Below it, an
implementation MUST evaluate the control at severity `warn`, which reports and does not block.

No control is born blocking. A rule that starts failing builds on the day it ships does not get
anything fixed; it gets the gate switched off, and takes every other control with it.

**`required_evidence`** names what proves the control. `manifest_field` means the control
verifies a declaration, which is legitimate — but a pack whose blockers are *all* declarations is
a form to fill in. An implementation SHOULD report that as a defect in the pack.

## 6. Authority

Every pack MUST declare `authority_status`:

| Value | Means |
|---|---|
| `binding_law` | a law in force in the declared jurisdiction |
| `regulatory_instrument` | issued by a regulator under a law |
| `draft` | proposed, not in force |
| `voluntary_standard` | a published standard nobody is compelled to follow |
| `best_practice` | this project's own view |

This exists so that a voluntary framework or an internal preference cannot be presented to a
team as a legal obligation. An implementation MUST surface it wherever it explains why a control
applies, and MUST NOT allow a lower authority to reduce a `binding_law` severity — see `03`.

Every pack MUST carry `reviewed_at`. External sources move faster than any package does, and a
mapping nobody has re-read is a mapping that may no longer be true.

## 7. Content restrictions on a distributed pack

A pack fetched from a registry MUST contain only `.yaml` and `.md` files. An implementation MUST
refuse an archive containing anything else, MUST refuse absolute paths, path traversal, symlinks
and device nodes, and MUST verify the declared checksum **before** writing anything to disk.

A checksum verified after unpacking is a checksum verified after the damage.
