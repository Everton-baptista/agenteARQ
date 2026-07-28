# 04. Expression language (normative)

Purpose: define the language a control's `check.expr` is written in.
Spec version: 1.0 · Status: draft

---

## 1. Why it is restricted

A pack is **data, never code**. Packs are distributed, shared through a registry, and written by
third parties. A governance standard that executes third-party code in order to verify
governance provides an execution primitive to anyone who can get a pack adopted — which is the
opposite of what it is for.

Therefore an implementation MUST NOT evaluate `check.expr` by delegating to a general-purpose
interpreter, a template engine that permits calls, or any facility that can reach the host. The
language below is total: every expression terminates, allocates boundedly, and can touch nothing
but the evaluation context.

An implementation MUST reject an expression that does not parse, rather than treating it as
false. A malformed check that silently passes is worse than no check.

---

## 2. Grammar

```
expr        := or_expr
or_expr     := and_expr ( "or" and_expr )*
and_expr    := unary ( "and" unary )*
unary       := "not" unary | comparison
comparison  := primary ( op primary )?
op          := "==" | "!=" | "<" | "<=" | ">" | ">=" | "in" | "not in"
primary     := literal | call | path | "(" expr ")"
call        := ident "(" [ expr ( "," expr )* ] ")"
path        := ident ( "." ident | "[]" | "[" NUMBER "]" )*
literal     := STRING | NUMBER | "true" | "false" | "null" | list
list        := "[" [ expr ( "," expr )* ] "]"
```

`STRING` is single- or double-quoted with backslash escapes. Comments are not supported.

---

## 3. Values and the context

Values are: null, boolean, number, string, list, map, and **multi**.

The evaluation context is a map. For a control evaluated against an agent it contains the
manifest under its natural field names, plus:

| Key | Contents |
|---|---|
| `agent` | the manifest's `agent` object |
| `tools` | the resolved `*.tool.yaml` documents, in manifest order, each under its `tool` key merged with the manifest's per-tool `approval` and `rate_limit` |
| `mcp_servers` | resolved entries from the MCP allowlist |
| `evals` | the parsed eval result, when one is referenced and readable |
| `now` | the current date, as `YYYY-MM-DD` |

---

## 4. Multi values

`[]` applied to a list yields a **multi** — every element, evaluated in parallel by the operators
that follow.

```
tools[].tool.effect                  → multi of every tool's effect
tools[].tool.effect == "irreversible" → multi of booleans
```

Rules:
- A binary operator with one multi operand yields a multi of the element-wise results.
- Two multis of the same length combine element-wise; of different lengths is an error.
- `[]` on null or on a non-list yields an **empty multi**, not an error. A control about tools
  must therefore hold vacuously for an agent with no tools, which is the correct reading.
- A multi reaching the top level without passing through `all` or `any` is an error. Requiring
  the author to say which one they meant removes an entire class of silently-wrong control.

---

## 5. Functions

The set is closed. An implementation MUST reject any other identifier used as a call.

| Function | Result |
|---|---|
| `all(x)` | true when every element of multi `x` is truthy; **true when empty** |
| `any(x)` | true when at least one element is truthy; **false when empty** |
| `len(x)` | length of a list, string, map or multi |
| `exists(x)` | true when `x` is not null, and — for lists, maps, strings, multis — not empty |
| `matches(s, re)` | true when string `s` matches regular expression `re` |
| `age_days(d)` | whole days between date `d` (`YYYY-MM-DD`, or an ISO 8601 timestamp) and `now` |
| `date(s)` | parses a date, for comparison against another date |
| `lower(s)` / `upper(s)` | case folding |

Regular expressions MUST be evaluated with a non-backtracking engine or an equivalent
linear-time guarantee. A pack that can supply a regular expression can otherwise supply a
denial-of-service.

`age_days` on a missing or unparseable date yields null; comparisons against null are false. A
control that must distinguish "stale" from "never recorded" checks `exists` first.

---

## 6. Truthiness and comparison

- Truthy: `true`, non-zero numbers, non-empty strings, non-empty lists and maps.
- Falsy: `false`, `0`, `""`, empty list, empty map, `null`.
- `==` and `!=` compare by value; a list equals a list with equal elements in order.
- Ordering operators apply to numbers and to dates. Applied to anything else they yield null,
  and a comparison yielding null is false.
- `in` tests membership in a list, a substring of a string, or a key of a map.

---

## 7. Limits

An implementation MUST enforce, and MUST report exceeding as an error rather than a failed check:

| Limit | Value |
|---|---|
| Expression source length | 4096 bytes |
| Parse depth | 64 |
| Multi cardinality at any point | 10000 |
| Total evaluation steps | 100000 |

---

## 8. Worked examples

```
# Every tool denies egress by default.
all(tools[].tool.permissions.network.deny_by_default == true)

# Above L1, nothing irreversible may run unapproved.
agent.autonomy.level in ["L0_suggest", "L1_act_with_approval"]
  or all(tools[].tool.effect in ["read", "write"]
         or exists(tools[].tool.approval.required_when))

# The model is pinned to an immutable identifier.
agent.model.pinned == true and not matches(agent.model.id, "(latest|current|stable)$")

# The eval result is fresh enough to mean anything.
exists(agent.evaluation.last_result_ref)
  and age_days(evals.completed_at) <= agent.evaluation.max_result_age_days

# All three guardrail points were considered.
exists(agent.guardrails.input) or len(agent.guardrails.input) == 0
```

---

## 9. Conformance

`spec/conformance/expr/` contains expression/context/result triples. An implementation claiming
`spec/1.0` MUST reproduce every expected result, including the errors: rejecting a malformed
expression and rejecting an unreduced multi are conformance requirements, not implementation
choices.
