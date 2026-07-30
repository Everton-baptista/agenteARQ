# Where to look

Read the file for the task at hand, not preemptively. Paths are under `agentarch/std/`.

| Task | Read |
|---|---|
| Agent scope, autonomy, budget, owner | `standards/01-agent-contract.md` |
| System prompts; RAG, grounding, citations | `standards/02-prompt-and-context.md` |
| Tools; permissions, timeouts, idempotency | `standards/03-tools.md` |
| Connecting or auditing an MCP server | `standards/04-mcp.md` |
| Memory, session state, tenant isolation | `standards/05-memory-and-state.md` |
| Multi-agent, planning, handoff, loop control | `standards/06-planning-and-multiagent.md` |
| Human approval flows | `standards/07-hitl.md` |
| Guardrails and fail modes | `standards/08-guardrails.md` |
| Prompt injection, exfiltration, sandboxing, secrets | `standards/09-security.md` |
| Personal data, redaction, retention | `standards/10-privacy.md` |
| Evals, datasets, thresholds, red team | `standards/11-evaluation.md` |
| Tracing, metrics, cost | `standards/12-observability.md` |
| Timeouts, retries, circuit breakers, budgets, SLOs | `standards/13-resilience-and-cost.md` |
| Releasing a change; what forces revalidation | `standards/14-lifecycle.md` |
| Model, dataset and dependency provenance; AI-BOM | `standards/15-supply-chain.md` |
| Agent as an HTTP API; edge, auth, caller budgets | `standards/16-service-and-edge.md` |
| Applying this to a specific framework | `adapters/<framework>.md` |
| Why a check failed | `agentarch explain <control.id>` |
