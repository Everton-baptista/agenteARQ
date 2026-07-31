"""The application. Three layers, and the dependency direction between them is a control.

  api/     transport. May import agent, domain, infra.
  agent/   the agent loop, prompts, tools, guardrails. May import domain, infra. Never api.
  domain/  business logic. No LLM, no HTTP. Imports nothing from here.
  infra/   database, cache, provider clients.

`agentarch check` enforces the "never api" line as control.ai.api.core_transport_separated.
"""
