# Adapter: CrewAI

Role-based agents in a crew. The framework encourages many agents with broad tool access, so tool least privilege and delegation budgets are the controls that do the work here.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

`role`, `goal` and `backstory` compose into the system prompt. Keep the composed result in a
versioned file and hash **that**, not the three fields separately: what the model receives is
what determines behaviour, and it is the composition that reaches the model.

## 2. Tools and the permission check

Assign tools per agent, never to the crew as a whole. A crew-wide tool list gives every agent
the union of everyone's authority, which is the opposite of least privilege and is the default
that costs the most.

## 3. The three guardrail points

CrewAI has no native guardrail hooks at the three points. Wrap tools for the action point, and
put input and output checks around `crew.kickoff`. Record in the manifest that these are
external to the framework — a control implemented outside the thing it protects is worth
knowing about during review.

## 4. Telemetry

Add OTel manually around task execution. Span per task, plus the delegation depth: `loop_rate`
and `human_intervention_rate` are the metrics that catch a crew talking to itself.

## 5. Handoff and approval

Delegation is implicit and model-driven, which is the risk. Set `max_iter` and
`allow_delegation` per agent from `autonomy.max_steps` and `handoff.hands_off_to` — an implicit
delegation graph has no declared authority and no termination argument.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
