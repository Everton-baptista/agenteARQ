# 08. Versioning and compatibility (normative)

Three independent semver lines. Separating them is what lets the reference implementation improve
without forcing every project to re-examine its rules.

| Line | Governs | A major means |
|---|---|---|
| `spec/x.y` | schemas, resolution, expression language, exit codes, shim rendering | an existing artifact may stop validating |
| `content/x.y.z` | standards, controls, packs, templates, adapters, skills | a severity was raised or a control removed |
| `cli/x.y.z` | the reference implementation | a flag or exit code changed meaning |

## Spec

A **minor** MAY add optional fields, add an enum value, add a function to the expression
language, add an exit code, or add a target. It MUST NOT make an existing valid document
invalid.

A **major** MAY do anything, and MUST be accompanied by a migration note.

An implementation MUST refuse an artifact whose `schema_version` major it does not implement, and
MUST exit 1 rather than attempting a best-effort read. Silently interpreting a document written
against a different contract is how a governance tool comes to report confidently on something
it misunderstood.

## Content

A **minor** MAY add controls, add packs, add or revise prose, and add translations. New controls
MUST enter with `enforced_from` at least one minor ahead, so they run in warn mode first.

A **minor MUST NOT** raise the severity of an existing requirement. That needs a major.

This is the promise that makes upgrading safe: a project can take a content minor without a
build breaking on a rule that passed yesterday. Without it, every upgrade becomes a negotiation
and projects pin forever.

A **major** MAY raise severities and remove controls. Removal MUST be preceded by at least two
minors with `status: deprecated`.

## CLI

Ordinary semver. The CLI MAY be ahead of the content a project has installed, and MUST use the
project's installed `agentarch/std` when one is present — a project pinned to an older content
release keeps being judged by that release, which is what makes upgrading a decision rather than
an event.

## Compatibility matrix

Each release publishes which spec majors and content ranges it supports. An implementation
SHOULD print all three versions on request; `version` output that names only the binary leaves
the reader unable to reproduce a result.

## Deprecation

A control marked `deprecated` MUST still be evaluated and MUST be reported as deprecated. It
MUST NOT be silently dropped: a project relying on it deserves to be told, and a control that
disappears without notice looks like a fixed failure.

## What is not versioned

Message wording, output layout, and the order of findings. An implementation may change these
freely, and anything scripting against them is relying on something no version promises. That is
why every command that CI is likely to consume offers a machine-readable format.
