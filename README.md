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

## Getting started

### 1. Install

If you have Node, there is nothing to install:

```bash
npx agentarch@latest start
```

That is the recommended path — no install step, no PATH to fix, and the version is pinnable
(`npx agentarch@0.1.3`). The npm package carries the platform binary as an
`optionalDependency`, the pattern esbuild and swc use, so npm's own integrity check covers the
executable you run. There is no `postinstall` fetching anything.

The CLI is one static binary, though, and **nothing here requires Node** — a Java or .NET project
should not acquire a JavaScript runtime to validate its agents. Without Node:

```bash
curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/install.sh | sh
```

It works out your platform, verifies the download against the signed `checksums.txt`, and puts
`agentarch` on your PATH. It refuses to install anything whose checksum does not match, and it
never uses `sudo` on your behalf — without write access to `/usr/local/bin` it installs to
`~/.local/bin` and tells you how to add it.

Piping a script into a shell is a decision, not a default. To read it first, which is the right
instinct:

```bash
curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/install.sh -o install.sh
less install.sh && sh install.sh
```

Other channels:

```bash
# Container — needs nothing on the host but Docker
docker run --rm -it -v "$PWD:/work" -w /work ghcr.io/everton-baptista/agentarch:latest start

# Go, if you have it
go install github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest

# Python
pipx install agentarch
```

Or take a signed binary straight from
[Releases](https://github.com/Everton-baptista/agenteARQ/releases) and verify it by hand:

```bash
gh release download v0.1.2 --repo Everton-baptista/agenteARQ \
  -p 'agentarch_0.1.2_darwin_arm64.tar.gz' -p 'checksums.txt'
shasum -a 256 -c checksums.txt --ignore-missing
tar -xzf agentarch_0.1.2_darwin_arm64.tar.gz
./agentarch version
```

Every release is signed with cosign keyless; the footer of each release page carries the
verification command, and the installer prints it too.

### 2. Start

```bash
mkdir my-agent && cd my-agent
agentarch start
```

That is the whole entry point — in an empty directory, plain `agentarch` does the same. It asks at
most five questions in plain language, skips any it can answer for itself, and derives the rest:

```
agentarch — let's get you set up.

A few questions in plain language; nothing is written until you say so.
Press Enter to take the default, or q to quit.

Is this a new agent, or does the code already exist?

  1. New — start me off with a complete project that works, so I can edit it
  2. Already built — describe what is here and tighten it up gradually

(this directory looks empty)
Choose 1–2 [1]:

What are you building?

  1. An agent that answers from my documents and cites its sources
     rag-support — runs on none, langgraph

  2. An agent that acts on my systems, with a human approving the dangerous part
     tool-approval — runs on none
  …

Where are the people who will use it?

  1. Brazil                     brings the LGPD rules in
  2. Europe                     brings GDPR and the EU AI Act in
  3. Both                       both sets apply
  4. Somewhere else, or not decided yet

Who is accountable for this agent?

A person, not a team. This is who decides to switch it off when it
misbehaves — a queue cannot make that call at 2am.

Name [Ana Silva]:
```

Then it shows exactly what will happen, waits for a yes, and does all of it: installs the
standard, writes a complete working project, generates the instruction files every assistant
reads, writes your answers into the manifests, and runs `validate` and `check` so you can see
it passes before you touch anything.

You never type the words *profile*, *jurisdiction* or *blueprint*. It picks up your name from
`git config`, notices whether the directory already has code, and only asks about a framework
when the starting point you chose ships more than one.

It never overwrites a file that already exists, and nothing is written before you confirm.

What lands on disk:

```
agentarch/
  agentarch.yaml        your settings — never overwritten by an upgrade
  std/                  the standard itself — replaced wholesale by `upgrade`
  project/              your manifests, tool specs, evals — never touched by an upgrade
app/                    runnable code, with the placeholders marked
AGENTS.md               generated, read by Codex, Cursor, Gemini CLI, Grok, Kimi, Zed, Aider
CLAUDE.md               generated, read by Claude Code
GEMINI.md               generated, read by Gemini CLI
.github/workflows/      the gate, already wired
```

**Commit the generated instruction files.** They are outputs: edit `agentarch/std/core/` and
re-run `sync`. CI checks they are current, so a hand-edited `CLAUDE.md` fails the pull request
instead of drifting quietly for six months.

Everything `start` does is also available one command at a time — see
[the manual route](#the-manual-route) below. And in a script or CI, where there is nobody to ask:

```bash
agentarch start --new --blueprint rag-support --framework none \
  --owner "Ana Silva" --jurisdictions BR --yes
```

### 3. Run the agent

```bash
python -m venv .venv && source .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

python app/agent.py "where is my order BR-77120?"
```

`app/README.md` explains what to read first and what to change. The short version: replace
`retrieve()` with your retriever, edit `out_of_scope` in the manifest, mirror it into the
prompt's refusal section, and replace the tools with yours.

### 4. Check it

```bash
agentarch validate      # structure and internal consistency
agentarch check         # the release gate
agentarch conformance   # L1 / L2 / L3, with an expiry
```

Exit codes are distinct so CI can route them:

| Code | Means | What to do |
|---:|---|---|
| 0 | passed | — |
| 2 | an artifact is malformed or inconsistent | read the finding; it names the field |
| 3 | a generated file is out of date | run `agentarch sync` |
| 4 | a blocker-severity control failed | `agentarch explain <control.id>` |
| 5 | a waiver expired | it belongs to the person named on it |
| 6 | a revalidation trigger fired | re-run evals, update `last_validated_at` |

When something fails, `agentarch explain <control.id>` gives the reasoning, the fix, and which
pack imposed it.

### 5. Wire it into CI

```yaml
name: agentarch
on: [pull_request]
jobs:
  agentarch:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Everton-baptista/agenteARQ/.github/actions/agentarch@v1
        with:
          command: sync --check
      - uses: Everton-baptista/agenteARQ/.github/actions/agentarch@v1
        with:
          command: check --profile standard
```

The blueprints ship this file already, at `.github/workflows/agentarch.yml`.

---

## The manual route

`start` is a composition of commands that all exist on their own. Use them directly when you are
scripting, adding a second agent, or want to see each step:

```bash
agentarch init --profile standard --jurisdictions BR   # install the standard
agentarch blueprint list                              # what starting points exist
agentarch blueprint show rag-support                   # what one demonstrates
agentarch blueprint add rag-support --framework none --yes
agentarch sync                                        # regenerate the instruction files
```

`agentarch blueprint` with no arguments is the same chooser `start` uses, without the rest of the
interview. `--jurisdictions` decides which regulatory packs apply — `BR` brings LGPD, `EU` brings
GDPR and the AI Act; leave it out if none apply.

---

## Already have agents?

Do not start over, and do not switch the gate off on day one. `agentarch start` detects existing
code and offers this path by default; the explicit form is:

```bash
agentarch init --adopt --profile standard
```

It scans for what already exists — providers, models, frameworks, likely prompt files — and
writes a manifest describing it. **Everything it could not determine is left as `unknown`**, on
purpose: a plausible-looking wrong value is worse than a blank one, because nobody re-examines a
field that is already filled in.

Fill in the unknowns, starting with `owner.accountable` and `out_of_scope`. Then:

```bash
agentarch check --adopt-baseline
```

That records today's failures as the starting point. From then on the gate blocks only what is
**new or worse**. Nothing is forgiven — `agentarch score` still counts the debt, and you close it
deliberately:

```bash
agentarch check --update-baseline   # drops entries you have fixed
```

---

## The rest of the commands

| | |
|---|---|
| `agentarch start` | the guided entry point — asks, then does all of the below |
| `agentarch new agent <id>` | scaffold an empty agent instead of using a blueprint |
| `agentarch new tool <id> --effect irreversible` | scaffold a tool, with its approval block |
| `agentarch mcp audit --probe` | has a server changed its tool descriptions since review? |
| `agentarch diff --base main` | which revalidation triggers fired, and is validation overdue |
| `agentarch report --out reports/` | markdown and a self-contained HTML page |
| `agentarch score` | maturity by dimension, declared vs proven; never blocks |
| `agentarch aibom --out ai-bom.json` | models, prompts, corpora, tools, MCP servers |
| `agentarch upgrade --dry-run` | what a newer standard would change here |
| `agentarch pack list --installed` | which packs are judging this project |

Every command takes `--root` to work on a directory other than the current one.

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
