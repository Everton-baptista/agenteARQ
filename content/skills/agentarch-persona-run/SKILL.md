---
name: agentarch-persona-run
description: Exercise the running product as each persona from personas.yaml through a browser-automation MCP server, from a host subagent, and report what a real user would trip over — as findings to reproduce, never as a pass.
metadata:
  version: "1.0.0"
---

# Run the personas against the product

Persona runs find problems that scripted tests did not think of. They are advisory: simulated
users are unreliable stand-ins for people, so a persona run never passes or blocks anything.

1. **Where things run.** Start the product locally or in the no-network sandbox. The persona
   agent is a subagent of your host (for Claude Code, `agentarch-persona-user`) that drives the
   product through a browser-automation MCP server. AgenteARQ itself never calls a model.
2. **Fix the run.** Record the model id, a step cap (25 by default), a time and token budget,
   and the number of runs per persona (3 by default). Keep traces without prompt or response
   content.
3. **Act as the persona.** For each persona, pursue its goal through the real interface only:
   no source code, no direct database or API access the persona would not have. Adversarial
   personas try what their description says.
4. **Report findings.** Each finding has the persona, what was attempted, what happened, what
   was expected, exact reproduction steps, a screenshot reference and "seen in k of n runs".
   Never report "no issues" as evidence that the product works.
5. **Promote only what reproduces.** Replay each finding with a plain browser test, no model
   involved. If it reproduces, write it as a failing test named after a new scenario and ask the
   owner to approve it; only then can it block. Findings that do not reproduce stay advisory.

Stop when every persona has run the agreed number of times or the budget is spent, and hand the
owner a short list in plain language: what a user would struggle with, ordered by severity.
