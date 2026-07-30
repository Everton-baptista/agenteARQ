## What this changes, and why

<!-- The failure it fixes or the gap it fills. Link the issue or RFC if there is one. -->

## Which version line moves

<!-- Delete the ones that do not apply. See spec/normative/08-versioning.md. -->

- [ ] `spec/x.y` — schemas, resolution, expression language, exit codes, shim rendering
- [ ] `content/x.y.z` — standards, controls, packs, templates, adapters, skills
- [ ] `cli/x.y.z` — the reference implementation
- [ ] none of them — docs, tests, CI

## Checks

```bash
go build ./... && go test ./...
go run ./cmd/agentarch sync --check --targets agents_md,claude,gemini
go run ./cmd/agentarch validate examples/01-rag-support-agent
go run ./cmd/agentarch check --profile standard examples/01-rag-support-agent
```

- [ ] The above pass locally.
- [ ] No generated file was hand-edited. Files whose header says `agentarch:generated` come from
      `agentarch sync`; if a shipped tree's shims changed, they were regenerated rather than
      patched.

## If this adds or changes a control

- [ ] An RFC exists (`rfcs/`) — required for a new control, a severity change, a new sync target,
      a schema change, or a new official pack.
- [ ] The prose half exists in `content/standards/`, anchored so `standard_ref` resolves.
- [ ] It enters with `enforced_from` one minor ahead. No control is born blocking.
- [ ] There is a failing example under `examples/99-failing/` with an `expected.yaml`. **A control
      with no failing example is a control nobody has watched fail.**
- [ ] It can actually fail — it does not re-check something a schema already guarantees.

## If this changes the spec

- [ ] A minor adds only optional fields, enum values or behaviour, and makes no existing valid
      document invalid.
- [ ] A conformance fixture covers it under `spec/conformance/`, so a second implementation does
      not have to read the Go.
- [ ] The English source and any translation move together, with `source_sha256` updated. A stale
      translation is worse than a missing one.
