---
name: agentarch-design-product
description: Design architecture, data and API contracts, and user journeys for an accepted software change, recording consequential tradeoffs and accessibility needs.
metadata:
  version: "1.0.0"
---

# Design the smallest sufficient architecture

Read the requirements and existing implementation before selecting components. Preserve
the project's stack unless a concrete requirement justifies changing it. Compare plausible
alternatives by operational complexity, failure isolation, compatibility, and maintainability.

Record consequential decisions with alternatives and consequences in an ADR. Define only
the contracts needed across actual boundaries: input/output, authorization, error behavior,
ownership of data, and compatibility. Avoid speculative abstraction layers.

For user interfaces, map essential journeys, empty/loading/error states, keyboard access,
focus, labels, and responsive behavior. Use existing design conventions; validate with a
real browser or platform tool when the implementation exists.

Identify threat boundaries, dependencies, and data migrations. Include verification and
recovery for the risks introduced by this design. Do not describe a prompt instruction as
an enforced security boundary. Stop when the design supports concrete implementation tasks.
