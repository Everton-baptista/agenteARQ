# 07. Conformance (normative)

Two different claims share the word, and conflating them is how "compliant" comes to mean
nothing.

- **Project conformance** — what a project that adopted the standard has achieved: L1, L2, L3.
- **Implementation conformance** — that a tool implements this specification correctly.

## Part 1 — project conformance

The levels are cumulative and ask three different questions.

### L1 Declared — are the agents described?

- every agent's manifest validates
- `owner.accountable` is non-empty for every agent
- `out_of_scope` has at least one entry for every agent
- `autonomy.level` and `stop_conditions` are declared
- the generated instruction files are in sync

### L2 Enforced — do the rules block?

Everything in L1, plus:

- the gate runs in continuous integration with `fail_on: [blocker]`
- no blocker-severity control is failing
- guardrails are declared at all three points for every agent

An implementation MUST determine "runs in CI" from a committed pipeline definition, not from
configuration or from a claim. A gate that exists only on a laptop is a linter; the whole content
of L2 is that the rules block a merge, which requires them to run where merges happen.

### L3 Proven — is there evidence rather than assertion?

Everything in L2, plus, for every agent:

- an evaluation result exists and is within `max_result_age_days`
- red team has been executed
- a threat model exists at `links.threat_model`
- telemetry is enabled with a pinned semantic-convention version

### Expiry

An L3 assessment MUST carry an expiry: the earliest date at which any piece of its evidence goes
stale, which in practice is the oldest eval result plus its freshness window.

After that date the assessment is L2. Nobody downgrades it — time does.

This is a MUST because conformance that never decays is advertising. A badge earned in March and
displayed in November describes a system that has not been measured since March.

An implementation MUST NOT report an expiry for L1 or L2, which rest on declarations rather than
on dated evidence.

### The badge

`conformance --badge` emits a shields.io endpoint document. An implementation MAY choose its own
colours and MUST report the level as the message, using a distinguishable value when no level is
reached.

An implementation MUST NOT report a level higher than the one computed, and MUST NOT provide an
option to do so.

## Part 2 — implementation conformance

An implementation may claim `agentarch spec/1.0` compliance when it:

1. reproduces every result in `spec/conformance/`, including the errors
2. implements the exit codes in `06` with their stated precedence
3. implements pack resolution per `03`, including the union rule, the highest-severity rule and
   the binding-law floor
4. enforces the budgets in `05` as errors rather than truncations
5. rejects, rather than executes, everything `04` says must be rejected

Point 5 is the one that matters most. An implementation that evaluated `check.expr` with a
general-purpose interpreter would satisfy every functional test and still be unsafe, because the
guarantee is about what a hostile pack **cannot** do.

An implementation MUST NOT claim compliance while extending the language, adding a check kind
that executes anything, or relaxing a MUST. It MAY add commands, output formats and diagnostics
freely — those are not part of the contract.

## Trademark

The name may be used to state a factual claim of compliance. It may not be used to imply
endorsement, or as the name of a derivative product. See `TRADEMARK.md`.
