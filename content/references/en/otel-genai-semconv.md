# OpenTelemetry GenAI semantic conventions → what to emit

Reviewed 2026-07-29 against semconv 1.29.0.

The conventions are still moving, which is exactly why `obs.semconv_pinned` exists: an unpinned
integration changes attribute names under you, and the dashboard goes quiet rather than red.

## Spans

| Span | When | agentarch adds |
|---|---|---|
| `gen_ai.invoke_agent` | one agent invocation | `agentarch.agent.id`, `agentarch.autonomy.level` |
| `gen_ai.execute_tool` | one tool call | `agentarch.tool.effect`, `agentarch.guardrail.decision` |
| `gen_ai.retrieve` | one retrieval | `agentarch.corpus.version` |

The additions are namespaced under `agentarch.` so they cannot collide with a future convention.

## Attributes worth emitting

| Attribute | Why |
|---|---|
| `gen_ai.system`, `gen_ai.request.model` | which provider and model actually ran |
| `gen_ai.usage.input_tokens`, `output_tokens` | cost, and the input that grew without anyone noticing |
| `gen_ai.response.finish_reason` | a truncated response looks like a bad answer |
| `agentarch.tool.effect` | lets you alert on irreversible calls specifically |
| `agentarch.guardrail.decision` | **the highest-signal event an agent produces** |

## What never to emit

`gen_ai.prompt` and `gen_ai.completion` carry the content. The conventions make them optional
for good reason, and `privacy.capture_content_default_off` makes them off by default here.

Enabling them turns an observability pipeline into a complete, long-lived copy of every
conversation, sitting in systems with different retention and access rules from the ones that
were assessed. It is the single easiest way to build the largest store of personal data in a
company by accident.

Record shapes instead: field present, length, pattern matched. That answers most debugging
questions without becoming a data store.

## The alert people forget

**Guardrail denial rate falling to zero.** It looks like an improvement and usually means a
check stopped running. Nothing errors, so nothing else will tell you.
