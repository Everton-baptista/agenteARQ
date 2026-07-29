---
agent_id: request-orchestrator
reviewed_at: "2026-07-20"
reviewer: alex.moreau
method: STRIDE, adapted for agents
---

# Threat model — request-orchestrator

## 1. Attack surface

Working assumption, from `standards/09-security.md`: an attacker will eventually get text of
their choosing into the context. The useful question is what a fooled model can reach.

| Untrusted input | Arrives via | Trusted? |
|---|---|---|
| Customer message | `question` variable | no |
| Help centre passages | RAG, `help-centre-public@2026-07-01` | no — anyone who can edit an article can write into the prompt |
| Tool results | `search_orders` | no |
| MCP tool descriptions | `help-centre-docs` | no — see `standards/04-mcp.md` |
| `customer_tier` | application | yes, set server-side from the session |

| Capability | Effect | Reachable damage |
|---|---|---|
| `search_orders` | read | order data for the authenticated customer only; identity resolved server-side |
| `notify_customer` | communication | a message the customer receives and cannot un-receive |

Egress: `orders-api.internal.example.com`, `messaging-api.internal.example.com`.
Secrets by name: `ORDERS_API_TOKEN`, `MESSAGING_API_TOKEN`. Neither is in the repository.

## 2. Threats and mitigations

| # | Threat | STRIDE | Mitigation | Control |
|---|---|---|---|---|
| 1 | Injection in a help centre article makes the agent promise a refund | Tampering | Refusal is in `out_of_scope` and in the prompt; no refund tool exists at all | `agent.scope_declared` |
| 2 | Injected text asks for another customer's orders | Elevation | The tool takes an order reference; the customer is resolved from the session, never from model output | `tool.least_privilege` |
| 3 | Exfiltration via a markdown image URL in the reply | Info disclosure | Output guardrail strips external image and link targets; egress allowlist covers the tool path | `tool.exfiltration_guard` |
| 4 | The MCP server changes a tool description after review | Tampering | `tool_description_sha256` plus `mcp audit --probe` in CI | `mcp.description_hash_pinned` |
| 5 | Coercion into sending an abusive or false message | Repudiation | `notify_customer` requires human approval; `on_timeout: deny` | `tool.irreversible_requires_approval` |
| 6 | Loop burns budget on a hostile input | DoS | `max_steps: 8`, `max_tool_calls: 12`, `usd_per_run: 0.15` | `agent.stop_conditions`, `agent.budget_bounded` |
| 7 | Confident answer with no source | Misinformation | Citation required; missing citation degrades to escalation | `rag.citation_required` |
| 8 | PII from an order appears in a trace | Info disclosure | `capture_content: false`; redaction profile `pii_default_v1` | `privacy.capture_content_default_off` |

## 3. Accepted risks

| Risk | Why accepted | Owner | Review |
|---|---|---|---|
| Injection screening is evadable | It is a cost-raiser, not a boundary. Containment is carried by isolation and tool least privilege, both of which block. | alex.moreau | 2026-10-20 |
| Red team found 2 of 60 attacks partially successful | Both produced an escalation rather than an action. Tracked as regression cases in `triage-redteam`. | alex.moreau | 2026-10-20 |

## 4. Revalidation

Any of `model_changed`, `system_prompt_changed`, `rag_corpus_changed`, `tool_added`,
`autonomy_raised`, `guardrail_disabled` or `mcp_server_added` invalidates this document.
