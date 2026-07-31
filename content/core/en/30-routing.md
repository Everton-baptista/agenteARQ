# Where to look

Read the file for the task at hand, not preemptively. Paths are under `agentarch/std/`, and a
bare `NN-name` below means `standards/NN-name.md`.

| Task | Read |
|---|---|
| Agent scope, autonomy, budget, owner | `01-agent-contract` |
| System prompts; RAG, grounding, citations | `02-prompt-and-context` |
| Tools; permissions, timeouts, idempotency | `03-tools` |
| Connecting or auditing an MCP server | `04-mcp` |
| Memory, session state, tenant isolation | `05-memory-and-state` |
| Multi-agent, planning, handoff, loop control | `06-planning-and-multiagent` |
| Human approval flows | `07-hitl` |
| Guardrails and fail modes | `08-guardrails` |
| Prompt injection, exfiltration, sandboxing, secrets | `09-security` |
| Personal data, redaction, retention | `10-privacy` |
| Evals, datasets, thresholds, red team | `11-evaluation` |
| Tracing, metrics, cost | `12-observability` |
| Timeouts, retries, circuit breakers, budgets, SLOs | `13-resilience-and-cost` |
| Releasing a change; what forces revalidation | `14-lifecycle` |
| Model, dataset and dependency provenance; AI-BOM | `15-supply-chain` |
| Agent as an HTTP API; edge, auth, caller budgets | `16-service-and-edge` |
| Doing one of these tasks step by step | `checklists/` |
| Applying this to a specific framework | `adapters/<framework>.md` |
| Why a check failed | `agentarch explain <control.id>` |
