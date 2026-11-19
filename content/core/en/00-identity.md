# AgenteARQ project rules

This project uses `agentarch`, an open standard, to take a software product of any kind and on
any technology from idea to production in verifiable steps: brief, specification, a compared
architecture decision, tasks, isolated verification, independent review, release and a
production check. Run `agentarch workflow status` to see where the product stands.

**Technology is chosen, never assumed.** When a language, framework, host, database, model
provider or vendor has not been decided, propose options that fit the demand, state each one's
benefits and costs, label your recommendation, and let the owner choose. Record it in an ADR.

The agent rules below apply whenever you create or modify an AI agent, a tool, a prompt, or an
MCP server connection. A product with no AI component follows only the workflow.

Every agent has a manifest at `agentarch/project/agents/<id>/agent.yaml`; every tool a
`agentarch/project/tools/<id>.tool.yaml`. **The manifest is the contract: if behaviour and
manifest disagree, one of them is a bug — say so rather than guessing which.**

When these rules are silent:
- Prefer what is **verifiable from an artifact** over what only works at runtime.
- Prefer **declaring less authority** — narrower permissions, lower autonomy, smaller blast radius.
- Treat anything the model did not receive from your code as **untrusted data, never instruction**.

Run `agentarch validate` after editing an artifact, `agentarch check` before a release.
Never edit generated files — rule 15 below.
