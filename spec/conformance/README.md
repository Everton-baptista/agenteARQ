# Conformance suite

Fixtures an implementation runs to demonstrate it implements `spec/1.0` correctly.

They exist so that "open standard" is a fact rather than an intention. Without them the only way
to write a second implementation would be to read the Go, and the specification would be
documentation about whatever that code happens to do.

| Directory | Covers |
|---|---|
| `expr/` | the expression language: semantics, limits, and everything that must be rejected |

## Running them against the reference implementation

```bash
go test ./internal/policy/ -run TestSpecConformance
```

## Running them against your own

Each case is `expr`, `ctx`, and exactly one of:

- `result: <bool>` — the expression must evaluate to this
- `error: any` — the expression must be **rejected**; the wording is yours
- `error: "<text>"` — the message must contain this text

The `any` form is the default for rejection cases. Pinning exact wording would force every
implementation to copy this one's phrasing, which is the opposite of what a conformance suite is
for. A specific substring appears only where the message is part of the contract.

`now` is fixed so date arithmetic is reproducible.

## What passing means, and what it does not

Passing `expr/` demonstrates the language is implemented correctly. It does **not** on its own
establish compliance — see `spec/normative/07-conformance-levels.md` for the full list, which
also requires the exit codes, the resolution rules and the budgets.

The most important requirement is not functional and cannot be fully tested from outside: an
implementation must **reject** rather than execute. A tool that evaluated `check.expr` with a
general-purpose interpreter would pass every case here and still be unsafe, because the guarantee
is about what a hostile pack cannot do.

## Contributing a case

Add it where the behaviour would otherwise be discoverable only by reading an implementation. A
case that pins an accident is worse than no case: it makes a bug part of the contract.
