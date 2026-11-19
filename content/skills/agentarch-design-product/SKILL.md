---
name: agentarch-design-product
description: Design architecture, data and API contracts, and user journeys for an accepted software change; propose technology options with their trade-offs and let the owner choose, recording the comparison in an ADR.
metadata:
  version: "2.0.0"
---

# Design the smallest sufficient architecture

Read the brief, the specification and the existing implementation first. The brief's
constraints (team skills, budget, hosting, data, compliance, deadline, any mandated
technology) decide which options are plausible. An existing stack is the default option to
keep, not a reason to skip the comparison.

## Propose, compare, let the owner choose

AgenteARQ is technology-agnostic. Never pick a language, framework, database, host, model
provider or vendor silently, and never present one as the default because it is popular or
familiar to you.

1. Derive two to four options that genuinely fit this demand. Include the current stack when
   one exists, and the simplest thing that could work (often no new technology at all).
2. For each option state, in the brief's terms, what it buys and what it costs: fit to the
   requirement, learning curve for this team, running cost, hosting and operations,
   lock-in and portability, security and data residency, ecosystem maturity, time to first
   result.
3. Give your recommendation and the reason, clearly labelled as a recommendation.
4. Stop and ask the owner to choose. A decision is the owner's when it changes cost, hosting,
   data location, vendor relationships or the skills the team must have.
5. Record the comparison and the owner's choice in the ADR: `## Context`, `## Options
   considered` (each option under `###` with Benefits and Costs), `## Decision` naming the
   chosen option and who chose it, `## Consequences`. `agentarch workflow` refuses to reach
   `planned` with an ADR that compares fewer than two options or decides on one it did not
   compare.

## Then design only what the choice needs

Define the contracts needed across actual boundaries: input/output, authorization, error
behavior, ownership of data, and compatibility. Avoid speculative abstraction layers.

For user interfaces, map essential journeys, empty/loading/error states, keyboard access,
focus, labels, and responsive behavior. Use existing design conventions; validate with a
real browser or platform tool when the implementation exists.

Identify threat boundaries, dependencies, and data migrations. Include verification and
recovery for the risks introduced by this design. Do not describe a prompt instruction as
an enforced security boundary. Stop when the design supports concrete implementation tasks.
