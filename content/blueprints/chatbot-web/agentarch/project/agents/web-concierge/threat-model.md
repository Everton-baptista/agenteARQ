---
agent_id: web-concierge
reviewed_at: "2026-07-29"
reviewer: alex.moreau
method: STRIDE, adapted for agents
---

# Threat model — web-concierge

## 1. Attack surface

Working assumption, from `standards/09-security.md`: an attacker will eventually get text of
their choosing into the context. The useful question is what a fooled model can reach. This
agent is reachable by anyone who can load the website's chat page, which is everyone — so the
unauthenticated-visitor case is the baseline, not an edge case.

| Untrusted input | Arrives via | Trusted? |
|---|---|---|
| Visitor message | the chat page, `question` variable | no |
| Knowledge base passages | RAG, `website-knowledge-base@2026-07-01` | no — anyone who can edit an article can write into the prompt |
| Tool results | `check_service_status` | no |
| MCP tool descriptions | `product-docs` | no — see `standards/04-mcp.md` |
| `customer_tier` | application | yes, set server-side from the session |

| Capability | Effect | Reachable damage |
|---|---|---|
| `check_service_status` | read | the service's own status page, nothing else |
| `email_follow_up` | communication | an email the visitor receives and cannot un-receive |

The web client (`web/`) is part of the surface in one specific way: whatever it ships is served
to every visitor, so a credential or a provider endpoint in it is public the moment it deploys.
`control.ai.api.no_client_side_model_access` checks it at the gate, and the service test suite
asserts it on every run — a control that depends on a reviewer opening view-source is not a
control.

Egress: `status.internal.example.com`, `mail-api.internal.example.com`.
Secrets by name: `STATUS_API_TOKEN`, `MAIL_API_TOKEN`. Neither is in the repository.

## 2. Threats and mitigations

| # | Threat | STRIDE | Mitigation | Control |
|---|---|---|---|---|
| 1 | A visitor pastes an injection into the chat | Tampering | Refusal is in `out_of_scope` and in the prompt; input guardrail blocks the obvious attempts before the model is called | `agent.scope_declared`, `genai.prompt_injection` |
| 2 | The page is edited to call the model provider directly, key in the bundle | Info disclosure | The page calls only `/v1/ask`; the gate lint and the test suite fail the build if a provider reference appears under `web/` | `api.no_client_side_model_access` |
| 3 | Injected text asks for another tenant's data | Elevation | No tool takes a tenant or visitor identifier; identity is resolved from the token, server-side | `tool.least_privilege` |
| 4 | The MCP server changes a tool description after review | Tampering | `tool_description_sha256` plus `mcp audit --probe` in CI | `mcp.description_hash_pinned` |
| 5 | Coercion into emailing the visitor something abusive or false | Repudiation | `email_follow_up` requires human approval; `on_timeout: deny`; the approver sees the full body in the chat's approval card | `tool.irreversible_requires_approval` |
| 6 | Scripted abuse through the public chat page burns budget | DoS | `max_steps: 8`, `max_tool_calls: 12`, `usd_per_run: 0.15`, and a per-caller daily run bound in the auth layer | `agent.stop_conditions`, `api.budget_per_caller` |
| 7 | Confident answer about pricing with no source | Misinformation | Citation required; missing citation degrades to escalation | `rag.citation_required` |
| 8 | Visitor text appears in a log or a trace | Info disclosure | `capture_content: false`; the access log records route templates, never bodies | `privacy.capture_content_default_off` |

## 3. Accepted risks

| Risk | Why accepted | Owner | Review |
|---|---|---|---|
| The demo token is a shared bearer token, not SSO | It is the stand-in that lets the blueprint run with no identity provider. The direction — tenant out of a verified credential — is what production keeps; the dict is what it replaces. | alex.moreau | 2026-10-29 |
| Injection screening is evadable | It is a cost-raiser, not a boundary. Containment is carried by isolation and tool least privilege, both of which block. | alex.moreau | 2026-10-29 |

## 4. Revalidation

Any of `model_changed`, `system_prompt_changed`, `rag_corpus_changed`, `tool_added`,
`autonomy_raised`, `guardrail_disabled` or `mcp_server_added` invalidates this document.
