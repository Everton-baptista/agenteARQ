---
lang: pt-BR
source: content/standards/en/13-resilience-and-cost.md
source_sha256: "ee70aaf08c83a2e244378866b10e71fa892cae15906cb9718ff10ae0d493aca6"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 13. Resiliência e custo

Propósito: falhar com elegância, e continuar viável ao fazê-lo.
Versão: 0.1 · Status: draft

An agent depends on a service that is slower, more variable and more expensive than anything else
in the stack, and that occasionally rate-limits you without warning. Everything here follows
from that.

Cost belongs in this standard rather than a business one because it is an availability property:
an attacker who cannot take a system down can make it expensive enough to be switched off.

---

## 1. Regras

### control.ai.agent.budget_bounded

**Intenção.** A run has a ceiling.
**Severidade** `major`

`usd_per_run`, `tokens_per_run`, `latency_p95_ms`. Derive them from an observed p95, then alert
above. A budget invented before any measurement is a number, not a bound.

### control.ai.agent.circuit_breaker

**Intenção.** A degraded dependency does not become a queue of retries.
**Severidade** `major`

Per provider and per tool. When it opens: fall back to a declared alternative model, degrade to
a narrower capability, or escalate — decided in advance and written in the manifest, not chosen
during the incident.

### control.ai.agent.graceful_degradation

**Intenção.** Partial service beats no service.
**Severidade** `major`

Levels, in order: full → no retrieval, answer from the prompt with lower confidence → refuse and
escalate → refuse and say so. Each level is a decision about what the agent still promises.

### control.ai.tool.timeout_declared

See `03-tools.md`. Timeouts cascade: a tool timeout above the agent's latency budget makes that
budget fiction.

---

## 2. Caching, and when it is dangerous

| Cache | Use when | Dangerous when |
|---|---|---|
| Exact prompt | identical inputs recur | the answer depends on time or on who is asking |
| Semantic | paraphrases recur | "similar" is not "equivalent" — near-miss answers are wrong answers |
| Tool result | the underlying data is slow-moving | it is not, and the agent acts on stale state |
| Provider prompt cache | long stable prefixes | the prefix contains another user's context |

The rule that prevents the worst outcome: **the cache key includes the identity scope.** A cache
keyed on prompt text alone will eventually serve one customer's answer to another.

Semantic caching deserves particular suspicion. The failure is silent and looks like a correct
answer to a slightly different question.

---

## 3. Rate limits and queues

Provider limits are shared across your whole account, so one agent's retry storm degrades every
other agent you run. Per-agent and per-tool rate limits are as much about isolation as about
cost.

Retry with exponential backoff and jitter. Retrying a non-idempotent tool without an idempotency
key turns one intended action into several — see `03-tools.md`.

---

## 4. Deve / não deve

| Deve | Não deve |
|---|---|
| Derive budgets from an observed p95 | Invent a number before measuring |
| Include identity in the cache key | Cache on prompt text alone |
| Decide degradation levels in advance | Improvise during the incident |
| Break the circuit per provider and per tool | Use one global breaker |
| Jitter the backoff | Retry in lockstep and synchronise the storm |
| Treat semantic cache hits as suspect | Trust similarity as equivalence |

---

## 5. Antipadrões observados

**The retry storm.** A provider slows down, every agent retries, the account rate-limits, and
now everything is down rather than one thing being slow.

**Cache keyed on prompt text.** Two customers ask the same question. One receives the other's
order details.

**Semantic cache serving near misses.** "How do I cancel?" answered with the returns policy.
Nothing errors; the metric is a cache hit.

**Timeout above the latency budget.** The tool takes eight seconds, the p95 target is six, and
the budget was never achievable.

**Degradation decided live.** Retrieval is down, and someone chooses in the moment whether the
agent should answer without sources.

---

## Referências externas

Revisado em 2026-07-28. Apenas mapeamentos; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
