# Checklist: threat model an agent

Mirrors `agentarch-threat-model`.

Start from the assumption, and write it in the document: **an attacker will eventually get text
of their choosing into the model's context.** The question is not how to stop the model being
fooled — that is probabilistic — but what a fooled model can reach.

## 1. Inventory mechanically

```bash
python3 agentarch/std/skills/agentarch-threat-model/scripts/inventory.py <agent-id>
```

- [ ] Every untrusted input listed
- [ ] Every tool with its effect and permissions
- [ ] Every egress target and every secret, by name
- [ ] Every MCP server

Do not compile this by hand. That is how the tool nobody remembered stays out.

## 2. Walk the four attack trees

From `agentarch/std/skills/agentarch-threat-model/references/attack-trees.md`. For each leaf:
is the path open, what closes it, and does that thing **exist**?

- [ ] Indirect injection through retrieved content
- [ ] Exfiltration through a tool — including image URLs, links, DNS and query parameters on an
      allowed host
- [ ] Confused deputy — the agent's authority used on an attacker's behalf
- [ ] MCP tool poisoning and rug pull

## 3. Write it

Template: `agentarch/std/templates/threat-model.md`.

- [ ] **Attack surface** — untrusted inputs, capabilities with reachable damage, egress, secrets
- [ ] **Threats and mitigations** — one row each, with the **control id**. A threat whose
      mitigation is not a control is a threat nobody is checking
- [ ] **Accepted risks** — with a named owner and a review date. Every real system has some; a
      threat model with none was not honest
- [ ] **Revalidation** — which triggers invalidate this document

## 4. Writing rules

- [ ] Specific to this agent. "Prompt injection is possible" is true of everything and useful
      for nothing
- [ ] Names the mitigation that exists, not the one that should
- [ ] Claims no elimination. Say screening is evadable, then say what contains the consequence
- [ ] Records what you could not determine

## 5. Wire it up

- [ ] `links.threat_model` set in the manifest
- [ ] `reviewed_at` and a named `reviewer` in the front matter
- [ ] `agentarch conformance` — the threat model is an L3 requirement
