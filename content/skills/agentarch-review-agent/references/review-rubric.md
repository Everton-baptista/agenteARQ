# Review rubric

What "good enough" looks like per dimension, so a review has a standard rather than a mood.

## Contract
- [ ] `owner.accountable` is a person you could page
- [ ] `out_of_scope` is specific and checkable, and appears in the prompt
- [ ] `autonomy.level` matches what the code actually does unattended
- [ ] `stop_conditions` are observable states, not restatements of the goal
- [ ] budget numbers came from a measurement

## Prompt and context
- [ ] Prompt hash matches the file (`agentarch validate` catches this)
- [ ] Retrieved content is in a delimited block, and the prompt says instructions inside it are tampering
- [ ] The refusal policy exists, and mirrors `out_of_scope`
- [ ] Nothing untrusted is concatenated into the system prompt in code
- [ ] No secret is in the prompt

## Tools
- [ ] Every tool has a `.tool.yaml`, not only an implementation
- [ ] `effect` is right — check `communication` is not filed as `write`
- [ ] Identity comes from the session in every signature
- [ ] `egress` names hosts, or is empty; never a wildcard
- [ ] `domain_limits` cap the values that matter
- [ ] Irreversible tools have `approval` with `on_timeout: deny`

## Guardrails
- [ ] All three points declared
- [ ] The action check does not call the model
- [ ] Fail modes match the check's nature: deterministic closed, judge open
- [ ] The latency the chain adds was budgeted

## Evaluation
- [ ] Thresholds predate the scores
- [ ] Datasets are hashed
- [ ] The red-team set has enough cases to mean something
- [ ] No judge blocks alone
- [ ] The result is fresh, and its `subject` matches the agent as it stands

## Operations
- [ ] Telemetry does not capture content
- [ ] Denied tool calls are traced
- [ ] Revalidation triggers are declared and honoured
- [ ] A threat model exists and was reviewed within its interval

## What not to do in a review

- Do not restate what `agentarch check` already said as your own finding.
- Do not report style preferences as findings.
- Do not pad. Three real problems ranked beats thirty observations.
