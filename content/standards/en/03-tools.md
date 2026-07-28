# 03. Tools

Purpose: how a capability is declared so that its blast radius is knowable before it is used.
Version: 0.1 · Status: draft · Scope: `*.tool.yaml`, one per tool.

A tool is where an agent stops producing text and starts changing the world. Everything that
matters about safety is decided here, not in the prompt — a prompt expresses an intention, and a
permission expresses a limit. Only one of those survives a successful prompt injection.

---

## 1. Rules

### control.ai.tool.contract_declared

**Intent.** The tool exists as an artifact, not only as a function.
**Severity** `blocker`

Every tool has a `.tool.yaml` with `input_schema`, an owner, and a description written for the
model. Permissions declared only in code cannot be reviewed in a diff, cannot be checked by the
gate, and cannot be reasoned about by someone who does not read that language.

Note that `description_for_model` is part of the attack surface: it is text the model treats as
authoritative. Never build it by interpolating anything untrusted.

### control.ai.tool.effect_classified

**Intent.** Classify what the tool does to the world before implementing it.
**Severity** `blocker`

| Effect | Meaning | Consequence |
|---|---|---|
| `read` | returns data, changes nothing | lightest handling |
| `write` | changes state, can be undone | sandbox required |
| `irreversible` | cannot be undone | approval required |
| `money` | moves value | approval required |
| `communication` | reaches a third party | approval required; cannot be recalled |

`communication` is separated from `write` because a sent message is irreversible in the way that
matters — the recipient has already read it. Teams routinely classify "send an email" as a
write, and then discover the distinction during an incident.

Raising an effect is a revalidation trigger (`tool_effect_raised`).

### control.ai.tool.irreversible_requires_approval

**Intent.** Nothing that cannot be undone happens without a human, unless bounded explicitly.
**Severity** `blocker` · **Fail mode** `fail_closed`

A tool with effect `irreversible`, `money` or `communication` declares `approval` with
`required_when`, `approver_role`, `timeout_s` and `on_timeout`.

`on_timeout` is `deny` or `escalate`. It is never `allow`: an unanswered approval request is not
consent, and a system that treats silence as approval has an approval *step*, not an approval
*control*.

**How to fix.** Add the `approval` block, or narrow the tool so it is no longer irreversible —
for example by writing to a queue a human drains, rather than acting directly.

### control.ai.tool.least_privilege

**Intent.** A successful injection reaches as little as possible.
**Severity** `blocker` · **Fail mode** `fail_closed`

`permissions.network.deny_by_default` is `true` and `egress` enumerates explicit hosts.
`data_access` names the narrowest set of tables or fields that works. `scopes` are the minimum.

Assume the model will be persuaded to call this tool with attacker-chosen arguments. Every
question about the tool then becomes: *what is the worst thing reachable from here?* Wildcards
in `egress` answer "anything", which makes the field decorative.

**Anti-fix.** Never widen a permission to make a failure go away. The failure is information
about the task being wrong-shaped.

### control.ai.tool.idempotency_declared

**Intent.** Retries are inevitable; their consequences should not be a surprise.
**Severity** `major`

`idempotency.idempotent` is declared. When it is `false`, `key_field` names the input field
carrying the idempotency key. Agents retry on timeout, and a non-idempotent tool without a key
turns one intended refund into three.

### control.ai.tool.timeout_declared

**Intent.** No tool call hangs a run.
**Severity** `major`

`limits.timeout_ms` is required. `max_retries` and `rate_limit_rpm` should be set. Timeouts
cascade: the tool timeout must be comfortably below the agent's `latency_p95_ms` budget, or the
budget is fiction.

### control.ai.tool.exfiltration_guard

**Intent.** A tool cannot be turned into an outbound channel.
**Severity** `blocker` · **Fail mode** `fail_closed`

Egress is enumerated. Beyond the obvious HTTP call, consider the channels that do not look like
network access: a URL assembled into a returned image or link, a DNS lookup of an
attacker-chosen hostname, a query parameter on a legitimate host, an error message echoed back
with attacker-controlled content in it.

---

## 2. Do / don't

| Do | Don't |
|---|---|
| Classify `effect` before writing the implementation | Add the classification afterwards to pass the gate |
| Enumerate `egress` hosts | Use a wildcard, or leave it empty while making calls |
| Return actionable errors ("order not found") | Return raw stack traces or upstream payloads |
| Resolve the user server-side from the session | Accept a customer id from model output |
| Add `domain_limits` such as a maximum amount | Rely on the prompt to keep values sensible |
| Support `dry_run` for anything irreversible | Test irreversible tools only in production shape |

---

## 3. Affected artifacts and fields

`*.tool.yaml`: `effect`, `idempotency.*`, `permissions.network.deny_by_default`,
`permissions.network.egress`, `permissions.data_access`, `permissions.scopes`,
`permissions.secrets`, `permissions.sandbox`, `limits.*`, `approval.*`, `failure.*`, `owner`,
`tests`.

`agent.yaml`: `tools[].ref`, `tools[].approval`, `tools[].rate_limit`.

---

## 4. Expected evidence

| Control | Evidence |
|---|---|
| `contract_declared`, `effect_classified`, `idempotency_declared`, `timeout_declared` | tool spec |
| `irreversible_requires_approval` | tool spec, plus an approval audit record once HITL is in place |
| `least_privilege`, `exfiltration_guard` | tool spec, plus an egress test proving a non-allowlisted host is refused |

---

## 5. Observed anti-patterns

**Permission widened to fix a failing call.** The tool needed one table, was granted the schema,
and now an injection that reaches it reaches everything. The original failure was a signal that
the task boundary was wrong.

**The customer id comes from the model.** The agent is asked to look up "the customer's orders",
so the tool takes a `customer_id` argument — which the model fills in, and which an injected
instruction can therefore choose. Identity comes from the session, server-side, always.

**Send-email classified as `write`.** Undoing a write is a database operation. Undoing a sent
message is an apology.

**Retry without an idempotency key.** The first call succeeded and the response was lost. The
agent, seeing a timeout, tried again.

**Errors that leak.** A tool returns the upstream error verbatim, including a request URL with a
token in it, and the model helpfully includes it in the reply.

---

## 6. External references

Reviewed 2026-07-28. Mappings only; the standard never reproduces their text.

- OWASP Top 10 for LLM Applications — *Excessive Agency*, *Improper Output Handling*,
  *Sensitive Information Disclosure*.
- MITRE ATLAS — techniques covering exfiltration via an ML-enabled system's own capabilities.
- NIST AI RMF 1.0 — *Manage*, for bounding the consequences of a capability rather than only its
  likelihood of misuse.
