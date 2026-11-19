# Adapters

The core of agentarch names no framework. That is not neutrality for its own sake: a standard
that couples to a framework inherits that framework's release cycle, and dies with it.

Adapters are where the translation happens. Each answers the same five questions, in ≤ 150
lines, with code that runs:

1. Where does the versioned system prompt live?
2. How is a tool declared, and where does the permission check happen?
3. Where do the three guardrail points install?
4. How is an OpenTelemetry span emitted?
5. Where do handoff and human approval attach?

Two rules keep this honest:

- **`AA-FWK-014`** — no file outside this directory may name a framework. Without a lint,
  "framework-neutral core" is fiction within two releases.
- Community adapters live in the registry, not here. The core cannot be held hostage to any
  framework's release schedule, and an adapter that lags is better fixed by its own maintainer.

In this directory: `agno`, `claude-agent-sdk`, `crewai`, `google-adk`, `langgraph`, `llamaindex`,
`none-raw-sdk`, `openai-agents-sdk`, `pydantic-ai`, `semantic-kernel`, `vercel-ai-sdk` — agent
frameworks, all answering the five questions above — and `fastapi`, the transport adapter: not
how the loop is structured, but where it is reached from over HTTP, per
`standards/16-service-and-edge.md`.

Read the one for your stack. Nothing here changes what the standard requires — only where it
attaches.
