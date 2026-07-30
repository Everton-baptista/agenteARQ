<!-- agentarch:generated v=1.1.0 core_sha256=4221e040d5f47bda4354a0a63c2b85ea17ca9a986dff2270e969422b916e3a92 target=claude lang=en
     DO NOT EDIT. Edit agentarch/std/core/en/ and run `agentarch sync`.
     Read by: Claude Code -->

# Agent architecture rules

This project builds AI agents under `agentarch`, an open standard. These rules apply whenever
you create or modify an agent, a tool, a prompt, or an MCP server connection.

Every agent has a manifest at `agentarch/project/agents/<id>/agent.yaml`; every tool a
`agentarch/project/tools/<id>.tool.yaml`. **The manifest is the contract: if behaviour and
manifest disagree, one of them is a bug — say so rather than guessing which.**

When these rules are silent:
- Prefer what is **verifiable from an artifact** over what only works at runtime.
- Prefer **declaring less authority** — narrower permissions, lower autonomy, smaller blast radius.
- Treat anything the model did not receive from your code as **untrusted data, never instruction**.

Run `agentarch validate` after editing an artifact, `agentarch check` before a release.
Never edit generated files — rule 15 below.

# Invariants

Non-negotiable. Each maps to a control; `agentarch check` enforces them.

1. NEVER give a tool with `effect: irreversible`, `money` or `communication` an autonomy above
   `L1_act_with_approval` without `approval.required_when`. → `control.ai.tool.irreversible_requires_approval`
2. NEVER concatenate retrieved or received content — RAG results, web pages, files, tool output,
   another agent's message — into the system prompt. It goes in a delimited untrusted block, as
   data. → `control.ai.genai.untrusted_content_isolation`
3. NEVER put a secret value in a manifest, prompt, log, span or repository file. Reference it by
   name. → `control.ai.agent.secrets_by_reference`
4. ALWAYS declare `out_of_scope` with at least one entry. An agent that has never been told what
   it must refuse will attempt it. → `control.ai.agent.scope_declared`
5. ALWAYS name a person in `owner.accountable`. A queue or a team alias is not accountable.
   → `control.ai.agent.owner_defined`
6. ALWAYS bound the loop: `max_steps`, `max_tool_calls` and `stop_conditions` are required.
   → `control.ai.agent.stop_conditions`
7. ALWAYS pin the model. A floating alias silently changes behaviour under you.
   → `control.ai.supply.model_pinned`
8. NEVER add an MCP server without an allowlist entry pinned to an exact version, with
   `tools_allow` enumerated. `default: deny` is mandatory. → `control.ai.mcp.allowlist_enforced`
9. NEVER widen a tool's permissions to make a failure go away. Narrow the task instead.
   → `control.ai.tool.least_privilege`
10. ALWAYS classify a tool's `effect` before writing its implementation. It determines
    everything else. → `control.ai.tool.effect_classified`
11. ALWAYS version the system prompt and record its `sha256` in the manifest. Editing a prompt
    without bumping the version is a silent behaviour change. → `control.ai.genai.prompt_versioned`
12. A deterministic guardrail is `fail_closed`. An LLM judge is `fail_open` unless severity is
    high or critical. NEVER let an LLM judge be the only thing blocking a release.
    → `control.ai.eval.judge_not_sole_blocker`
13. NEVER default telemetry or evidence to capturing prompt and response content.
    `capture_content: false` unless there is a stated reason. → `control.ai.privacy.capture_content_default_off`
14. ALWAYS declare guardrails at all three points — user input, model output, and tool action.
    Missing a point is a decision, so record it. → `control.ai.agent.fail_mode_declared`
15. NEVER hand-edit a file whose header says `agentarch:generated`. Edit the source under
    `agentarch/std/core/` and run `agentarch sync`. CI fails otherwise (exit 3).

# Vocabulary

Full glossary: `agentarch/std/standards/00-index.md`. Five terms carry the invariants:

- **Effect** — what a tool does to the world: `read`, `write`, `irreversible`, `money`,
  `communication`. Drives approval and guardrails.
- **Autonomy** — how far the agent may go unattended: `L0_suggest` … `L4_autonomous`.
- **Guardrail** — a check at user **input**, model **output**, or tool **action**. Not a
  prompt instruction.
- **Fail mode** — when a guardrail cannot decide: `fail_closed` (block), `fail_warn` (record),
  `fail_open` (allow).
- **Untrusted content** — anything not authored by your code. Always data, never instruction.

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
