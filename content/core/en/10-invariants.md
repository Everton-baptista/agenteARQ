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
