# Governance

How agentarch changes, and what adopters can rely on.

A standard that one party can change at will is a product with a marketing problem. This
document exists so that adopting agentarch is a decision about engineering rather than a bet on
a maintainer's continued goodwill.

---

## 1. What is normative

| Layer | Contents | Changed by |
|---|---|---|
| **Spec** (`spec/`) | schemas, expression language, resolution, exit codes, conformance levels | RFC, plus a conformance-suite update |
| **Content** (`content/`) | standards, controls, packs, templates, adapters | RFC for anything that can fail a build; pull request otherwise |
| **Implementation** (`cmd/`, `internal/`) | the reference CLI | ordinary pull request |

The reference implementation is not the standard. Where they disagree, the spec is right and the
implementation has a bug. `spec/conformance/` exists so a second implementation can prove itself
correct without reading a line of the Go.

---

## 2. When an RFC is required

Open one in [`rfcs/`](rfcs/) for:

- a new control, or a change to an existing control's expression
- raising a severity, or shortening a grace period
- a new sync target
- any schema change
- a new official pack, or a change to what a profile includes
- a new normative language

Not required for: prose edits that do not change a rule, new examples, adapters, translations,
bug fixes, tests.

An RFC states the problem, the rule proposed, **how it is verified**, the cost of adoption, and
what happens to projects that already adopted. That last section is the one that gets skipped
and the one that matters: every control ships onto systems already in production.

---

## 3. Control lifecycle

```
proposed → experimental → stable → deprecated → removed
```

| Stage | Meaning |
|---|---|
| `proposed` | an open RFC; not shipped |
| `experimental` | shipped, evaluated only under `--profile experimental` |
| `stable` | in an official pack |
| `deprecated` | still evaluated, reported as such, removal announced |
| `removed` | gone; a pack referencing it fails to load |

**No control is born blocking.** A control enters a pack with `enforced_from` set one minor
version ahead and runs in warn mode until then. A rule that starts failing builds the day it
ships does not get anything fixed — it gets the gate switched off, and takes every other control
with it.

**No release raises the severity of an existing control without a content major.** Adopters can
take a minor upgrade without a build breaking on a rule that passed yesterday.

**Deprecation runs for at least two minor versions** before removal.

---

## 4. Versioning

Three independent semver lines, with a published compatibility matrix:

| Line | Major means | Minor means |
|---|---|---|
| `spec/x.y` | a schema or algorithm change that breaks existing artifacts | additive fields, new optional behaviour |
| `content/x.y.z` | a severity was raised, or a control removed | new controls in warn mode, new packs, new standards |
| `cli/x.y.z` | a flag or exit code changed meaning | new commands and flags |

The CLI refuses content whose `spec_version` major it does not implement, and exits 1 rather
than guessing. Silently interpreting an artifact written against a different contract is how a
governance tool comes to report confidently on something it misunderstood.

---

## 5. Deciding

Today: one maintainer, decisions in public pull requests and RFC threads.

This is stated plainly rather than dressed up as a committee. A single maintainer is a real risk
for anyone adopting, and the mitigations are structural rather than promissory:

- spec and content are **CC BY 4.0** — forkable, translatable, reimplementable, with attribution
- the **conformance suite** makes a second implementation practical
- the **registry** carries community packs and adapters without a merge to this repository
- packs are **data**, so an organisation can add its own rules without touching the core

If maintenance lapses, everything above continues to work and a fork loses nothing.

The intent is to move to a small group of maintainers with public voting once there are
contributors with sustained investment. That change will itself be an RFC.

---

## 6. Compatibility promises

For as long as `spec/1.x` is current:

1. Exit codes keep their meanings. New ones may be added; existing ones do not move.
2. Manifest fields are not removed or repurposed. New fields are optional.
3. Control IDs are not reused. A removed ID stays removed.
4. `agentarch upgrade` never touches `agentarch/project/` or `agentarch.yaml`.
5. Generated files carry the digest of the source they came from, so drift is always detectable.

---

## 7. Security

Report vulnerabilities per [SECURITY.md](SECURITY.md), not in a public issue.

One class gets priority over everything else: **anything that makes a pack able to execute
code**. Packs travel through a registry and come from third parties, and the entire trust model
rests on them being inert data. `spec/conformance/expr` treats that as a conformance
requirement, not an implementation detail.
