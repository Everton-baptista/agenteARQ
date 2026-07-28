# Vocabulary

Use these words with these meanings. Precision here prevents whole classes of design error.

- **Agent** — a system that chooses actions to reach a goal. If it cannot call a tool, it is a
  prompt, not an agent.
- **Manifest** — `agent.yaml`. The declared contract of one agent. Source of truth.
- **Tool** — a capability with a typed contract and declared permissions. Defined in a
  `.tool.yaml`, never only in code.
- **Effect** — what a tool does to the world: `read`, `write`, `irreversible`, `money`,
  `communication`. Drives approval and guardrails.
- **Autonomy** — `L0_suggest`, `L1_act_with_approval`, `L2_act_reversible`,
  `L3_act_irreversible_bounded`, `L4_autonomous`. How far the agent may go unattended.
- **Guardrail** — a check at one of three points: user **input**, model **output**, tool
  **action**. Not a synonym for prompt instruction.
- **Fail mode** — what happens when a guardrail cannot decide: `fail_closed` (block),
  `fail_warn` (allow, record), `fail_open` (allow).
- **Untrusted content** — anything not authored by your code: user text, retrieved documents,
  web pages, tool results, other agents' output. Always data, never instruction.
- **Control** — one verifiable rule, `control.ai.<type>.<name>`. Has prose and an executable check.
- **Pack** — a versioned set of controls with severities. Data, never code.
- **Profile** — which packs apply here: `minimal`, `standard`, `regulated`.
- **Gate** — `agentarch check`. Blocks a release on `blocker` severity.
- **Waiver** — a time-boxed, owned exception. Maximum 90 days, never permanent.
- **Handoff** — one agent transferring work to another, with typed payload, declared authority,
  return point and timeout.
- **Revalidation trigger** — a change that invalidates prior assurance: model, system prompt,
  RAG corpus, provider, new tool, raised autonomy, disabled guardrail, new MCP server.
- **Evidence** — an artifact proving a control holds: eval result, test, hash, attestation.
  Distinct from a declaration.
- **Shim** — a generated assistant instruction file (`AGENTS.md`, `CLAUDE.md`, …). Output, never input.
