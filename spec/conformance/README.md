# Conformance suite

Fixtures an implementation runs to demonstrate it implements `spec/1.0` correctly.

They exist so that "open standard" is a fact rather than an intention. Without them the only way
to write a second implementation would be to read the Go, and the specification would be
documentation about whatever that code happens to do.

| Directory | Covers | Requirement in `07` |
|---|---|---|
| `expr/` | the expression language: semantics, limits, and everything that must be rejected | 1, 5 |
| `exit-codes/` | the codes, and the precedence between them when more than one condition holds | 2 |
| `resolution/` | pack selection, applicability, the union rule, the highest-severity rule and the binding-law floor | 3 |
| `budgets/` | the rendering budgets, and that exceeding one is an error rather than a truncation | 4 |

Together these cover the five requirements in `spec/normative/07-conformance-levels.md` Part 2.

## Running them against the reference implementation

```bash
go test ./internal/policy/ ./internal/render/ -run TestSpecConformance
```

## Running them against your own

Every directory is a YAML file of cases with an `id`, an `about` where the reason is not obvious,
and an expectation. What varies is what the case describes.

**`expr/`** — `expr`, `ctx`, and exactly one of:

- `result: <bool>` — the expression must evaluate to this
- `error: any` — the expression must be **rejected**; the wording is yours
- `error: "<text>"` — the message must contain this text

The `any` form is the default for rejection cases. Pinning exact wording would force every
implementation to copy this one's phrasing, which is the opposite of what a conformance suite is
for. A specific substring appears only where the message is part of the contract.

`now` is fixed so date arithmetic is reproducible.

**`exit-codes/`** — `conditions`, the set of things that hold when a command finishes, and the
single `exit_code` that must be reported. Most cases list more than one condition, because each
code is obvious on its own and the ordering between them is the part an implementation invents.

**`resolution/`** — a self-contained `catalogue` of packs and controls, the `profile_packs`, the
`agent` fields resolution reads, and an `expect.resolved` map of control id to severity. Carrying
its own catalogue is deliberate: no case depends on the content any implementation ships.

Two things are deliberately not asserted. The **order** of the resolved set, because `03` §5 says
order must not affect results and pinning one would make a parallel implementation
non-conforming for no reason. And the **wording** of any explanation, because `03` §4 requires
that a resolution can be explained, not how.

**`budgets/`** — a `target`, a core size, and whether rendering must succeed or fail. Sizes are
given either as a number or as a keyword (`renders_at_budget`, `renders_one_over`,
`core_equals_budget`), because the budget bounds the **rendered file** and the header is not a
fixed cost any implementation can be asked to hard-code. Measure your own overhead and size the
core from it.

## What passing means, and what it does not

Passing every directory demonstrates that the functional half of
`spec/normative/07-conformance-levels.md` Part 2 holds: the language, the exit codes, the
resolution rules and the budgets.

It is not the whole claim. The most important requirement is not functional and cannot be fully
tested from outside: an implementation must **reject** rather than execute. A tool that evaluated
`check.expr` with a general-purpose interpreter would pass every case here and still be unsafe,
because the guarantee is about what a hostile pack cannot do, and a suite can only demonstrate
what a well-formed one does.

So passing is necessary and not sufficient. What it buys is that a second implementation can be
written from `spec/` without reading anyone's source — which is the entire reason the licences
permit it and `GOVERNANCE.md` offers it as the answer to a project with one maintainer.

## Contributing a case

Add it where the behaviour would otherwise be discoverable only by reading an implementation. A
case that pins an accident is worse than no case: it makes a bug part of the contract.
