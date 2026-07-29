# Why this exists, and what it is not

## The problem

Building a responsible AI agent means assembling knowledge from a dozen places that do not
reference each other: OWASP's LLM risk list, MITRE ATLAS, NIST's risk framework, ISO/IEC 42001,
the OpenTelemetry GenAI conventions, the prompt-injection literature, MCP security advisories,
and whatever your framework calls a "tool".

None of it is executable. So each team invents its own conventions, the knowledge lives in one
person's head, and nothing survives a change of framework, of assistant, or of team.

Meanwhile the AI assistant became the primary author of agent code, and it starts every session
with no memory of what the team decided. `AGENTS.md` and its siblings helped, and then multiplied
into seven hand-written files that diverge within weeks.

## What agentarch is

A **standard**: normative contracts, executable controls, and a CLI that checks them. It reads
artifacts and never enters an agent's execution path, which is what keeps it usable from Python,
TypeScript, Go, Java or .NET without becoming a dependency of any of them.

## What it is not

**Not a runtime.** It never executes an agent and never imports an agent framework. The pressure
to add "just a small execution helper" arrives eventually; taking it would end the portability
that makes the rest work.

**Not a certification.** `conformance` reports what a project has achieved against evidence it
produced. It is not an audit, and the badge expires precisely so nobody mistakes it for one.

**Not a replacement for judgement.** Every control is a rule someone wrote down. A project can
satisfy all of them and still have built the wrong thing.

## The three decisions everything else follows from

**A pack is data, never code.** Packs travel through a registry and come from third parties. A
governance tool that executed them would hand an execution primitive to anyone who can get a pack
adopted. This is why the expression language exists, and why it is total and closed.

**Every rule exists twice.** As prose someone can read, and as a control something can check,
under one identifier, verified in both directions. Prose with no verifiable consequence is an
opinion; a check with no explanation is an obstacle. Requiring both means a rule must survive two
different questions — *can you justify this?* and *can you detect it?* — and many plausible rules
survive neither.

**The core is a fixed budget.** What every assistant loads on every session is capped, and the
build fails when it overflows. Adding an invariant means removing one, which makes "what is truly
non-negotiable" a scarce and contested decision instead of a growing list nobody reads.

## What would make this a failure

Being adopted and changing nothing. A project can fill in every field, pass every control, and
have exactly the same agent it had before. The defences against that are structural rather than
hopeful:

- at least one blocker per pack must rest on an artifact that had to be produced
- `score` reports declared and proven separately, and never blocks
- conformance L3 rests entirely on evidence, and expires
- a waiver needs an owner and a date, and lapsing produces its own exit code

If those stop being true, the standard has become paperwork and should be abandoned rather than
maintained.
