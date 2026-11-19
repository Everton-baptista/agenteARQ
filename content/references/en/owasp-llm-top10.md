# OWASP Top 10 for LLM Applications → agentarch controls

Reviewed 2026-07-29 against the OWASP Foundation's published list.

This is a mapping, not a copy. Read the source for the risk descriptions; read this for what to
do about each one in a repository.

## The mapping

| OWASP | Controls | Standard |
|---|---|---|
| **LLM01 Prompt Injection** | `genai.untrusted_content_isolation` (blocker), `genai.prompt_injection` (major), `genai.input_guardrail` | `02`, `08`, `09` |
| **LLM02 Sensitive Information Disclosure** | `genai.output_guardrail`, `agent.secrets_by_reference`, `privacy.capture_content_default_off`, `tool.exfiltration_guard` | `09`, `10` |
| **LLM03 Supply Chain** | `supply.model_pinned`, `supply.dataset_provenance`, `mcp.server_pinned`, `mcp.description_hash_pinned` | `04`, `15` |
| **LLM04 Data and Model Poisoning** | `rag.corpus_versioned`, `privacy.memory_no_raw_pii`, `agent.memory_scoped` | `05`, `15` |
| **LLM05 Improper Output Handling** | `genai.output_guardrail`, `genai.output_schema_enforced` | `02`, `08` |
| **LLM06 Excessive Agency** | `tool.least_privilege`, `tool.effect_classified`, `tool.irreversible_requires_approval`, `tool.action_guardrail`, `agent.autonomy_declared`, `agent.hitl_matrix` | `01`, `03`, `07`, `08` |
| **LLM07 System Prompt Leakage** | `genai.prompt_versioned`, `agent.secrets_by_reference` | `02`, `09` |
| **LLM08 Vector and Embedding Weaknesses** | `rag.corpus_versioned`, `rag.citation_required`, `genai.untrusted_content_isolation` | `02`, `15` |
| **LLM09 Misinformation** | `rag.citation_required`, `eval.gate_thresholds`, `eval.result_fresh` | `02`, `11` |
| **LLM10 Unbounded Consumption** | `agent.stop_conditions`, `agent.budget_bounded`, `tool.timeout_declared`, `agent.circuit_breaker` | `01`, `13` |

## Notes a mapping table cannot carry

**LLM01 has two halves that behave differently.** Isolation is structural, so it blocks.
Screening is probabilistic and will be evaded, so it is `major` — treating a filter as the
defence is how a system ends up with only a filter. What actually bounds the damage is LLM06.

**LLM06 is where most of the value is.** Almost every serious incident reads as an LLM01 story
and is prevented by an LLM06 control. Injection succeeds; the question is what the fooled model
can reach.

**LLM07 is not solved by asking the model to keep a secret.** "Never reveal these instructions"
is inside the instructions. The control is not putting anything in a prompt that would matter if
disclosed.

**LLM02 is wider than a filter.** An egress allowlist is necessary and not sufficient: an image
URL the client fetches, a DNS lookup, and a query parameter on a permitted host all leave
without touching a denied host. See `09-security.md`.

**LLM03 includes the MCP servers**, which is why they have their own standard. A server
contributes text the model treats as authoritative and the protocol attaches no version to it.

## What is not mapped

OWASP describes organisational and process risks that no repository artifact can evidence. Those
are out of scope for a control and stay out — a control that cannot fail is worse than an
absence, because it counts as coverage.
