---
name: agentarch-test-scenarios
description: Turn a specification's acceptance criteria into executable scenarios and the brief's users into personas, then map every scenario to a deterministic test in the product's own stack, so "tests pass" means the product does what the owner meant.
metadata:
  version: "1.0.0"
---

# Write scenarios and personas

Run this after the specification is accepted and before planning tasks.

1. **One or more scenarios per criterion.** For every `AC-n` in `spec.md`, write scenarios in
   `agentarch/project/scenarios.yaml` (template: `agentarch/std/templates/scenarios.yaml`). Each
   has a stable id, the criterion it covers, a `kind` (`happy`, `failure`, `permission` or `edge`)
   and `given`, `when`, `then` in words the owner can read. Every criterion needs at least one
   happy and one failure or permission scenario where such a case exists. Do not invent business
   rules; ask the owner.
2. **Personas.** Write `agentarch/project/personas.yaml` (template:
   `agentarch/std/templates/personas.yaml`) from the brief: goal, familiarity, device,
   accessibility needs. Keep the fixed adversarial personas from the template (invalid input,
   abuse, keyboard-only, screen reader, and prompt injection when the product has AI).
3. **Map each scenario to a test the verifier runs.** Choose the narrowest layer that proves it,
   in the product's own stack: unit, API contract (schema-driven fuzzing when an OpenAPI or
   GraphQL schema exists), property-based tests where a library exists, browser journeys with
   automated accessibility checks in the same run for interfaces. Name the scenario id in the
   test, so every scenario has a test and every test names a scenario.
4. **Keep tests honest.** A test generator or healer may repair a locator; it must never change
   an assertion or skip a test to make it pass. A failing test the owner did not approve to change
   is a finding, not something to fix by editing the test.

Mobile journeys need an emulator or a device, which a generic container does not have: declare
them as a separate profile rather than skipping them. Stop when every criterion has scenarios,
every scenario names its test, or when a business rule needs the owner.
