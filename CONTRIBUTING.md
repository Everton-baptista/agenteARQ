# Contributing

## Before anything else

Run the checks. They are fast, need no secrets and no network:

```bash
go build ./... && go test ./...
go run ./cmd/agentarch sync --check --targets agents_md,claude,gemini
go run ./cmd/agentarch validate examples/01-rag-support-agent
go run ./cmd/agentarch check --profile standard examples/01-rag-support-agent
```

## The one rule that shapes everything

**Every rule exists twice**: as prose in `content/standards/`, and as an executable control in
`content/packs/controls/`, under one identifier. `validate` checks the correspondence in both
directions — an undocumented control fails, and so does a documented rule that nothing verifies.

If you cannot write the check, write `check.kind: manual_attestation` and say why. That is an
honest declaration. What does not go in is prose with no consequence.

## Adding a control

1. Open an RFC (see [GOVERNANCE.md](GOVERNANCE.md) for when one is required).
2. Write `content/packs/controls/<type>/<name>.yaml`. Every control carries an `intent` that
   says what goes wrong without it, and a `remediation` that says what to do. A finding with no
   fix trains people to ignore findings.
3. Write the prose section, anchored so `standard_ref` resolves.
4. Add it to a pack with `enforced_from` one minor ahead. No control is born blocking.
5. Add a failing example under `examples/99-failing/` with an `expected.yaml`. **A control with
   no failing example is a control nobody has watched fail.**

Before proposing it, check that it can actually fail. A control that re-checks something a
schema already guarantees can never fire on a valid project — it makes the catalogue look
stronger than it is. This happened once already; see the git history for
`irreversible_requires_approval`.

## Adding a sync target

The `Targets` registry in `internal/render/render.go`, plus a conformance fixture. Every target is
a promise the standard keeps working in that tool, so it needs an RFC.

## Translations

English is normative. A translation carries front matter recording the SHA-256 of the source it
was made from, and `validate` reports drift as `AA-I18N-016`.

A stale translation is worse than a missing one: a missing one sends the reader to the English
source, while a stale one answers their question with authority using a rule that has since
changed. Update `source_sha256` when you retranslate, and never edit a translation to say
something the source does not.

Control IDs, schema fields and file names stay in English in every language, so error messages
and searches remain interoperable across teams.

## Writing style

Look at an existing standard before writing a new one. In short: numbered sections, heavy use of
tables, no emoji, no superlatives. Every anti-pattern is a failure mode with a mechanism, not a
style preference. Say what breaks and why, then say what to do.

## Scope

agentarch is docs, schemas, a CLI and CI. It is **not** a runtime. It never executes an agent
and never imports an agent framework. The pressure to add "just a small execution helper"
arrives eventually and would end the portability that makes this work in any language.

## Code

Go, `gofmt`, no linter config beyond `go vet`. Dependencies are kept minimal and pinned —
a governance tool with a large dependency tree is making an argument against itself.

## Conduct

[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). The review process here is adversarial on purpose —
"can this control actually fail?" and "who starts failing when it ships?" are meant to be hard
questions. They are asked of the proposal, never of the person who made it.
