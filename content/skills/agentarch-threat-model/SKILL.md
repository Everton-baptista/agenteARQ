---
name: agentarch-threat-model
description: >-
  Write or update a threat model for an AI agent. Use when the user asks for a threat model,
  security review or risk assessment of an agent; when a revalidation trigger fired and the
  existing one is stale; or when conformance L3 is failing on the threat model requirement.
---

# Threat modelling an agent

Start from one assumption, and say so in the document: **an attacker will eventually get text of
their choosing into the model's context.** Through a user message, a retrieved document, a tool
result, an MCP tool description, or another agent's output.

That reframes the exercise. Not *how do we stop the model being fooled* — prevention there is
probabilistic — but *what can a fooled model reach*. The second question has verifiable answers.

## 1. Inventory the surface mechanically

```bash
python3 agentarch/std/skills/agentarch-threat-model/scripts/inventory.py <agent-id>
```

It extracts, from the manifest and tool specs: every untrusted input, every tool with its effect
and permissions, every egress target, every secret by name, every MCP server. Do not compile
this by hand — you will miss the tool nobody remembered.

## 2. Walk the attack trees

`references/attack-trees.md` has the four that apply to nearly every agent:

- indirect injection through retrieved content
- exfiltration through a tool, including the channels that do not look like network access
- confused deputy, where the agent's authority is used on an attacker's behalf
- MCP tool poisoning and rug pull

For each, answer concretely for *this* agent: is the path open, what stops it, and is what stops
it a control that exists or an intention someone stated?

## 3. Write it

Use the template at `agentarch/std/templates/threat-model.md`. Sections:

**Attack surface** — two tables. Untrusted inputs and whether each is trusted. Capabilities with
effect and reachable damage. Plus egress and secrets by name.

**Threats and mitigations** — one row per threat: what it is, STRIDE category, the mitigation,
and the **control id**. A threat whose mitigation is not a control is a threat nobody is
checking, and saying so is the point of the column.

**Accepted risks** — with a named owner and a review date. Every real system has some. A threat
model with none is a threat model that was not honest.

**Revalidation** — which triggers invalidate this document.

## 4. Rules for the writing

- **Be specific to this agent.** "Prompt injection is possible" is true of everything and useful
  for nothing. "An injected instruction in a help centre article could make the agent promise a
  delivery date the order lookup did not return" is a threat someone can act on.
- **Name the mitigation that exists**, not the one that should. If nothing mitigates it, that is
  the finding.
- **Do not claim elimination.** Injection screening is evadable; say so, and say what contains
  the consequence instead.
- **Record what you could not determine.** A threat model with an honest gap is worth more than
  one that guessed.

## 5. Wire it up

Set `links.threat_model` in the manifest, and record `reviewed_at` and a named `reviewer` in the
front matter. Then:

```bash
agentarch validate
agentarch conformance
```

The threat model is one of the L3 requirements, so this is visible in the level.
