# agentarch

**An open, versioned, verifiable standard for building AI agents.**

Install it into any agent project, in any language, and every AI assistant working on that
project — Claude Code, Gemini CLI, Cursor, Copilot, Codex, Grok, Kimi, Qwen Code, Windsurf,
local models — follows the same architecture rules, from a single source of truth.

> Status: **pre-release, under active development.** `spec/1.0` is not frozen yet.
>
> 16 standards · 39 controls · 9 packs · 11 framework adapters · `en` and `pt-BR`.

---

## The problem

Building a responsible AI agent today means assembling knowledge that lives in a dozen
disconnected places: the OWASP Top 10 for LLM Applications, MITRE ATLAS, NIST AI RMF,
ISO/IEC 42001, the OpenTelemetry GenAI semantic conventions, the prompt-injection literature,
MCP security advisories, and whatever your framework happens to call a "tool" or a "guardrail".

None of it is executable. So every team reinvents its own conventions, the knowledge lives in
one person's head, and nothing survives a change of framework, of assistant, or of team.

Meanwhile the AI assistant became the primary author of agent code — and it starts every
session with no memory of what your team decided. Instruction files (`AGENTS.md`, `CLAUDE.md`,
`.cursor/rules`) helped, but every tool reads a different file, all written by hand, all
drifting apart within weeks.

## What agentarch does

It answers three questions that have no standard answer today:

1. **What must be declared** for an agent to count as well-built — in machine-readable
   artifacts, not prose.
2. **How that is verified** automatically, in CI, without running the agent.
3. **How every AI assistant** picks up those rules from one source of truth.

```bash
agentarch init --profile standard --jurisdictions EU,BR
agentarch validate            # structure and consistency        (exit 2)
agentarch check               # the release gate                 (exit 4, 5)
agentarch mcp audit --probe   # has a server changed since review?
agentarch diff --base main    # which revalidation triggers fired (exit 6)
agentarch conformance --badge # L1 / L2 / L3, with an expiry
```

`init` writes an `agentarch/` directory into your project and generates the instruction file
each assistant expects. `sync --check` runs in CI, so a hand-edited `CLAUDE.md` fails the pull
request instead of silently drifting for six months.

## What it is not

Not a library. Not a runtime. It does not execute your agent and does not replace your
framework. It has to work the same in Python, TypeScript, Go, Java and .NET — so it stays out
of the execution path entirely.

---

## How it is organized

Four layers, versioned and licensed separately, so that this can be a standard rather than
just a tool:

| Layer | What it is | Version | License |
|---|---|---|---|
| **Spec** | normative contracts: schemas, control and pack format, resolution algorithm, exit codes, shim rendering | `spec/1.0` | CC BY 4.0 |
| **Content** | the standards, controls, official packs, templates, adapters | `content/1.x` | CC BY 4.0 |
| **Implementation** | `agentarch`, the reference CLI, written in Go | `cli/1.x` | Apache-2.0 |
| **Governance** | RFC process, conformance levels, versioning policy, registry | continuous | — |

`spec/conformance/` holds fixtures and expected outputs, so anyone can write a second
implementation — in Rust, in TypeScript, inside an internal platform — and prove it correct.

### Every rule exists twice

Once as **prose** you can read (`content/standards/`) and once as an **executable control**
(`content/packs/controls/`), sharing an identifier. `validate` checks the correspondence in
both directions: an undocumented control fails, and so does a documented rule that nothing
verifies.

That is the core defense against becoming shelfware: **prose without a verifiable consequence
does not get into the standard.** A rule that genuinely cannot be automated is admitted as
`check.kind: manual_attestation` — an honest declaration, not a loophole.

### Two things that are never negotiable

- **A pack is data, never code.** Checks are expressed in a restricted expression language
  specified in `spec/normative/04-expression-language.md` — no `eval`, no arbitrary calls. A
  governance standard that executes third-party code to verify governance does not hold up.
- **The core is a fixed budget, not a list.** What every assistant loads on every session is
  capped, and the build fails when it overflows. Adding an invariant means removing another —
  which makes "what is truly non-negotiable" a scarce, contested decision.

---

## What is in the box

| | |
|---|---|
| **Standards** | agent contract, prompt and context, tools, MCP, memory, multi-agent, human-in-the-loop, guardrails, security, privacy, evaluation, observability, resilience and cost, lifecycle, supply chain |
| **Packs** | `core.agent`, `sec.owasp-llm`, `obs.otel`, `eval.baseline`, `reg.gdpr`, `reg.br-lgpd`, `reg.eu-ai-act`, `std.nist-ai-rmf`, `std.iso-42001` |
| **Adapters** | LangGraph, OpenAI Agents SDK, Claude Agent SDK, Google ADK, Pydantic AI, LlamaIndex, CrewAI, Semantic Kernel, Agno, Vercel AI SDK, and no framework at all |
| **Generated for** | `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `QWEN.md`, Cursor, Copilot, Windsurf, `.mcp.json` |

## Conformance

`agentarch conformance --badge` reports one of three levels:

| Level | Means |
|---|---|
| **L1 Declared** | agents are described: manifest, named owner, explicit out-of-scope, declared autonomy and budget |
| **L2 Enforced** | the rules block: gate in CI, guardrails at all three points, least-privilege tools, MCP allowlist denying by default |
| **L3 Proven** | there is evidence: evals within their freshness window, red team executed, threat model reviewed, OTel with pinned semconv, AI-BOM |

The badge **expires**. An L3 badge whose evals went stale drops to L2 on its own. Conformance
that never decays is advertising.

---

## Regulation is optional and pluggable

Standards never cite law. Legal obligations live in optional versioned packs — `reg.eu-ai-act`,
`reg.gdpr`, `reg.br-lgpd`, `std.iso-42001`, `std.nist-ai-rmf` — each declaring its authority,
its `authority_status`, and its review date. Your agent declares `jurisdictions: ["EU", "BR"]`
and the applicable packs resolve automatically.

This is what lets a team in Berlin, São Paulo or Austin share the same core.

---

## Language

English is normative. Translations declare the SHA-256 of the source they were made from, and
`validate` flags them when they fall behind — a stale translation is worse than a missing one,
because it lies with authority. Control IDs, schema fields and file names stay in English in
every language, so error messages and searches remain interoperable across teams.

Shipping in v1: `en`, `pt-BR`.

---

## Contributing

New controls, severity changes, new sync targets, schema changes and new official packs go
through the RFC process in [`rfcs/`](rfcs/). See [CONTRIBUTING.md](CONTRIBUTING.md) and
[GOVERNANCE.md](GOVERNANCE.md).

No control is ever born blocking. Controls enter with `enforced_from` one minor ahead and run
in warn mode until then, and no release makes an existing control stricter without a content
major.

## License

Code is Apache-2.0 ([LICENSE](LICENSE)). Spec and content are CC BY 4.0
([LICENSE-CONTENT](LICENSE-CONTENT)) so they can be quoted, translated and reimplemented.
