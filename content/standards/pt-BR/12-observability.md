---
lang: pt-BR
source: content/standards/en/12-observability.md
source_sha256: "63236f0d4e1ec2e4ea0679b2026113e2cb3159bb40ff1382af84fde40d830002"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 12. Observabilidade

Propósito: o que emitir, e o que nunca emitir.
Versão: 0.1 · Status: draft

An agent that cannot be observed cannot be debugged, and after an incident the only evidence is
someone's recollection. The constraint is that the obvious way to make an agent observable — log
everything — builds the largest store of personal data in the system.

---

## 1. Regras

### control.ai.obs.otel_enabled

**Intenção.** Telemetry another tool can read.
**Severidade** `major`

Spans per invocation, per tool call, per retrieval. Correlated with the application's existing
trace, so an agent failure is visible in the same place as the request that caused it.

### control.ai.obs.semconv_pinned

**Intenção.** Your dashboards keep meaning what they meant.
**Severidade** `major`

`semconv_version` is recorded. The GenAI semantic conventions are still moving; an unpinned
integration changes attribute names under you, and the dashboard goes quiet rather than red.

### control.ai.obs.tool_calls_traced

**Intenção.** The interesting events are the ones that did not happen.
**Severidade** `major`

Every tool call: name, effect, duration, outcome, and the guardrail decision. A **denied** call
is the highest-signal event an agent produces and the easiest to lose, because nothing went
wrong.

### control.ai.obs.cost_tracked and control.ai.obs.slo_declared

Cost per run against its declared budget — total spend moves for legitimate reasons and hides
the case that matters. SLOs cover availability, latency p95 and **task success rate**; the last
is the one that catches a system that is up, fast, and giving worse answers.

### control.ai.privacy.capture_content_default_off

`blocker` here too. See `10-privacy.md`.

---

## 2. What to emit

| Span | Attributes |
|---|---|
| `invoke_agent` | agent id, model, provider, tokens in/out, cost, latency, stop reason |
| `tool_call` | tool id, effect, guardrail decision, duration, outcome |
| `retrieval` | corpus id and version, `top_k`, results returned |
| `guardrail` | point, control id, verdict, fail mode |
| `approval` | tool, requested at, decided at, decision |

Never: prompt text, response text, retrieved passage bodies, tool arguments containing personal
data, or anything from `permissions.secrets`.

The trap is that argument values are exactly what you want when debugging. Record shapes and
identifiers instead — `order_reference` present, length 12, matched pattern — which answers most
debugging questions without becoming a data store.

---

## 3. Metrics worth alerting on

| Metric | Catches |
|---|---|
| cost per run vs budget | denial of wallet, a prompt that grew |
| guardrail denial rate | an attack in progress, or a guardrail that broke |
| escalation rate | a corpus gap, or a task the agent should not have |
| task success rate | quality regression with everything green |
| p95 latency | a guardrail chain nobody budgeted |
| tool error rate | a dependency degrading |

Guardrail denial rate falling to zero is the alert people forget. It looks like an improvement
and usually means a check stopped running.

---

## 4. Deve / não deve

| Deve | Não deve |
|---|---|
| Pin the semconv version | Track whatever the SDK emits today |
| Trace denied tool calls | Trace only what executed |
| Alert on cost per run | Alert on monthly spend |
| Record shapes and identifiers | Record argument values to debug |
| Correlate with the application trace | Keep agent telemetry in its own silo |
| Alert on denials falling to zero | Treat a quiet metric as good news |

---

## 5. Antipadrões observados

**Content capture for debugging.** Enabled during an incident, never turned off.

**Denied calls not traced.** The system looks clean; the only record of a blocked attack is a
counter nobody graphs.

**Cost measured monthly.** A single agent burning its budget forty times over is invisible until
the invoice.

**Attribute names drifted.** The provider integration updated, the dashboard shows nothing, and
nothing alerted because nothing errored.

**Task success unmeasured.** Availability, latency and error rate all green, answers steadily
worse.

---

## Referências externas

Revisado em 2026-07-28. Apenas mapeamentos; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
