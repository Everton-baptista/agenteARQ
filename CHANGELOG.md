# Changelog

Three version lines move independently — see `spec/normative/08-versioning.md`. Each entry says
which one changed.

## Compatibility matrix

`08-versioning.md` requires each release to publish which spec majors and content ranges it
supports, and `agentarch version` prints all three lines so a reported result can be reproduced.

| CLI | Spec majors implemented | Content shipped | Content it can read |
|---|---|---|---|
| 0.3.x | `spec/1` | `content/1.2.0` | `content/1.x` |
| 0.1.x | `spec/1` | `content/1.0.0` | `content/1.x` |

`0.2.0` has an entry below and was never tagged; its work shipped inside `0.3.0`.

The CLI may be ahead of the content a project has installed, and uses the project's own
`agentarch/std` whenever one is present — a project pinned to an older content release keeps
being judged by that release. A `schema_version` major the CLI does not implement is refused with
exit 1 rather than read on a best-effort basis.

## Unreleased

`content` — the blueprints gain a provider seam; `cli` gains the question that drives it.

**The provider is asked rather than assumed.** Every blueprint was Anthropic, and not only in the
manifest: `app/infra/provider.py` imported the Anthropic SDK and the runner called the Messages API
shape directly, so anyone on OpenAI or Google received a project that did not run. The seam is now
`create_message()` — one call shape, with a module per provider under `app/infra/providers/`
translating the two things that actually differ: how a tool is declared (`input_schema` ·
`parameters` inside `function` · `function_declarations`) and how the response is read (`content`
blocks · `tool_calls` on the message · `functionCall` in parts). Nothing in `app/agent/` changed,
which is the claim, and `app/tests/test_provider.py` is what checks it — with a fake client, so the
tests still run in a clone with no credentials in it.

The provider arrives as a parameter, read from `model.provider` by the caller that already read the
manifest. Reading it inside `infra` would invert the import direction
`control.ai.api.core_transport_separated` exists to keep one-way, and a provider chosen by an
environment variable is a model decision no review ever saw.

`agentarch start --provider <id>`, or the new question after "what are you building", writes the
answer to the three files that have to agree: `model.provider` and a pinned `model.id` in every
manifest, and the marked SDK line in `app/requirements.txt`. A manifest naming one provider beside a
requirements file pinning another's SDK validates, passes the gate, and fails at the first model
call — so CI now asserts all three together.

The pinned ids live in one table in `cmd/agentarch/provider.go` with a review date, and
`TestProviderTableIsReviewed` fails six months after it. This is the only part of agentarch that
goes stale on its own: nothing here can tell when another company retires a model, and a pinned id
that has been discontinued is worse than a floating alias because it looks like a decision.

The Anthropic row records what the blueprints already ship — `claude-sonnet-4-5-20250929` — rather
than the newest model, so choosing the default changes nothing. Moving the blueprints to a newer
Anthropic model touches the manifests, the examples and the conformance cases together, and belongs
in its own change rather than riding along in this one.

`PRICE_PER_MTOK` gained a row per provider. A model with no row costs zero, and a cost of zero makes
`autonomy.budget.usd_per_run` a limit that can never be reached — switching provider must not
quietly disable the spend cap.

`.env.example` now names all three credentials, values omitted as before.

## v0.3.0 — 2026-07-31

`cli/0.3.0` · `content/1.2.0` · `spec/1.0` gains one optional field.

**The interview asks which language, and the answer reaches every assistant.** `--lang` existed,
generated pt-BR instruction files, and was a flag nobody discovered — so everyone got the English
interview and English rules without learning there was a choice. It is now the first question,
asked in both languages because there is no chosen language yet in which to ask it, and the answer
feeds `--lang` through to `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `QWEN.md`, Cursor, Copilot and
Windsurf. A Portuguese team's assistant reads the rules in Portuguese, whichever assistant it is.

Scope stops at the interview. Gate findings, `explain` and `report` stay in English: translating
those puts a translation obligation on every new control, and a stale translation of a rule answers
the reader with authority using a rule that has since changed — `AA-I18N-016` in the tool rather
than in the standards.

The catalogue gained `order` and `i18n`. Sorting by the English `need` put the menu in alphabetical
order of a sentence, which is an accident rather than a decision, and once translated the same
catalogue reordered itself per language — so "pick the third one" meant different things to two
people looking at the same screen.

**The checklists were unreachable by the tools they were written for.** Skills install to
`.claude/skills/`; the checklists exist so the standard does not work better in one tool than
another, and `content/checklists/README.md` says so, naming Gemini CLI, Copilot, Codex, Grok, Kimi
and Qwen Code. But the routing table in the core had eighteen rows and none of them pointed at
`checklists/`, so the file every other assistant reads never mentioned they existed. The mechanism
was built and unreachable by its own audience.

Fixing it cost bytes the core did not have: eleven free on Windsurf in pt-BR, and a routing row
costs about seventy. The remedy is the one `05-shim-rendering.md` prescribes — shrink rather than
raise the budget. Declaring the `standards/` prefix once instead of repeating it in sixteen rows
paid for the new row nine times over: Windsurf went from 11 bytes of headroom to 98, Copilot from
33 to 120.

**A third way in, for code that already exists.** `--adopt` describes what is there; `--refactor`
installs the procedure for changing it. agentarch does not rewrite anybody's code — it is not a
runtime and it stays out of the execution path — so what ships is `agentarch-refactor` as a skill
and `checklists/refactor.md` for everything that does not load skills, and the assistant already
reading the project carries it out.

The procedure is four gates per slice: the tests pass, the gate is no worse than the baseline
recorded before anything moved, `validate` is clean, and no secret is in the diff. It says plainly
what it does not do — four gates catch a regression the tests cover, and catch neither a behaviour
nobody tested nor a design that satisfies every control and still serves the user badly. A green
gate on a broken refactor is the most expensive artifact in this standard.

**A seventh blueprint, and memory is why it exists.** `agentic-product` puts the goal, the tools,
the approvals and the observability in one service — but the part no other blueprint shows is an
agent that remembers, because remembering is what turns isolation into a problem that spans turns.

`app/agent/memory.py` prevents both failures structurally rather than carefully: every key is built
from a frozen `Principal.tenant_id` resolved server-side, and `recall()` has no parameter for a
tenant, so there is no argument an injected instruction could supply to read somebody else's
conversation. Every write carries the retention from the manifest — there is no `put()` without a
TTL — and `forget()` exists because "it expires in thirty days" is not an answer to somebody
exercising a right to erasure. Recalled facts go into a delimited untrusted block: a fact written
on turn 3 and recalled on turn 9 is a delayed-action injection, and memory is where invariant 2 is
easiest to forget because memory feels like ours in a way a retrieved page does not.

**CI had been red for two commits, and the second failure was hiding behind the first.** The
`examples behave as documented` job failed on its SARIF assertion, which stopped the nineteen
steps after it from ever running — including the one asserting the reference example reaches L3.
It did not: `examples/01-rag-support-agent` had been left with shims generated from an older core
when the repository's own were regenerated, so it failed L1 on "the generated instruction files
are in sync" and `conformance` reported **none** while CI reported nothing at all.

Both are fixed, and both now fail at the commit that causes them. `TestNoCommittedShimIsStale`
walks every shipped tree and runs `sync --check` over the targets actually committed there, so a
core edit that strands a shim fails on the machine that made it rather than in a CI step deep
enough to be masked.

**Four findings were being reported about a service that did not exist.** The SARIF assertion
read `results[0].level == 'error'` and got a `note`, because findings were ordered by control ID
and `examples/99-failing/unpinned-model` — a project with no HTTP interface and one floating model
alias — collected four warn-mode `api.edge` findings that sorted ahead of the blocker.

Two things were wrong. SARIF now orders most-severe-first, so a consumer reading one result sees
what blocks. And the four findings should never have existed: `02-control-and-pack.md` §4 already
says an inapplicable control must be skipped, but no `applies_to` condition could express "reached
over HTTP" — `system_type` records what an agent **is**, not how it is reached, and any of its six
values can sit behind an interface. RFC 0002 adds `applies_to.declares`, a closed list of optional
manifest sections a control needs present before it has anything to ask.

The five service controls take `declares: [interface]`. The two blockers do not, and that is the
load-bearing half: they read the repository, not the interface, and a committed `.env` is a public
credential in a library and a batch job exactly as in a service. Narrowing those would be noise
with the sign reversed — a control quietly not running where it was needed.

Folding the condition into `check.expr` would have needed no spec change and was rejected: it
reports a **pass** rather than a skip, so every agent without an interface would have earned five
free controls toward its maturity score. Manufacturing compliance out of absence is the thing this
project exists to prevent, and it was the cheap option.

**`Summarize` counted a skipped control as a pass**, which the same §4 forbids. `unpinned-model`
reported "33 evaluated · 32 passed" while six of those were never evaluated at all. It now reports
27 evaluated, 26 passed, and names the six.

**Three contract inconsistencies, each one a promise the project was not keeping.** `api` joined
the closed control vocabulary in content 1.1 and reached `control.schema.json` but not
`agent.manifest.schema.json`, so a manifest declaring a guardrail against `control.ai.api.*` was
rejected by the schema written to accept it; `waivers.schema.json` matched `[a-z]+` and would
accept a waiver against a type that cannot exist. One vocabulary, now checked identical in all
three. `TRADEMARK.md` was cited twice by the normative spec as the document governing the
compliance claim and had never been written — the one question a second implementer has to answer
before publishing. And `status: deprecated` was a MUST in `08-versioning.md`, parsed onto
`Control`, and read by nothing but `explain`; it now reaches the gate, the report and the SARIF
message.

`version` prints all three lines, and reports the content a project has installed rather than what
the binary shipped with — a project pinned to an older release is judged by that release, so which
one was used is the line that changes the answer. The compatibility matrix `08-versioning.md` and
`GOVERNANCE.md` both promise per release is published above.

**The conformance suite covered one of the five requirements it exists for.**
`07-conformance-levels.md` Part 2 lists five things a `spec/1.0` implementation must do;
`spec/conformance/` held fixtures for the expression language and nothing else. That matters more
than it looks: `GOVERNANCE.md` §5 states plainly that this project has one maintainer and offers
the suite as the structural mitigation — *"the conformance suite makes a second implementation
practical"* — which was true for a fifth of the contract.

`exit-codes/`, `resolution/` and `budgets/` join it: 81 cases where there were 59. The precedence
rule moved out of an if-chain inside `check` into `policy.ExitCode`, because it is normative and a
rule restated per command drifts between commands without anyone deciding it should. Every
resolution case carries its own catalogue, so no fixture depends on the content an implementation
ships, and neither order nor message wording is asserted anywhere — `03` §5 says order must not
affect results, and pinning one would make a parallel implementation non-conforming for no reason.

`CODE_OF_CONDUCT.md`, issue templates and a pull-request template fill the gap where
`CONTRIBUTING.md` and `GOVERNANCE.md` invite people into an RFC process the repository gave them
no way to enter.

## v0.2.0 — 2026-07-29

`cli/0.2.0` · `content/1.1.0`. Spec gains one control type; `spec/1.0` is unchanged otherwise.

**The blueprints are services now.** They ran as `python app/agent.py "question"`, which nobody
delivers to a customer — and the gap mattered because it is where the controls that already existed
stopped being true. The prompt leaked through the access log, because a web framework logs request
bodies by default. The memory `scope_key` came from a caller identity the standard did not describe,
so two tenants shared a store while every control passed. `usd_per_run` bounded one run and said
nothing about a caller who issued ten thousand. The commonest way to break invariant 3 is a
committed `.env`, and nothing checked. Invariant 2 was one f-string away from broken in a route
handler.

Three layers, with the dependency direction between them as the load-bearing rule: `agent/` may not
import `api/`. That is what keeps the loop runnable from a test, a queue worker or `app/cli.py`, and
the drift it prevents is gradual and always locally reasonable — a handler needs one header, the
runner imports the request, and a few weeks later the agent only runs inside a web server. The
LangGraph variant of rag-support differs from the no-framework one in exactly three files —
`agent/runner.py`, `requirements.txt` and `README.md` — which is that claim demonstrated rather than
asserted. (This entry said "two" until v0.3.1; `diff -rq` says three.)

What became real rather than declared: human approval is a queue with a TTL, a tenant check, single
use and an audit line, because the approver is not in the request and blocking a worker on a person
is how a service falls over under its own approvals. Caller identity comes from a verified
credential and no request field influences it. The access log records route template, status,
duration and tenant, and never content. Spans and cost metrics are emitted, with the same cost
figure enforcing the budget and feeding the dashboard. Retry has jitter, a circuit breaker, and a
rule that only idempotent work is repeated — never a tool call. Secrets resolve through one
function. Storage is a four-method protocol with the Redis implementation written out, because
in-memory is a deployment choice to make knowingly rather than a TODO to find in production.

**The blueprints shipped evidence they had invented, and conformance believed it.** groundedness
0.94, a jailbreak rate of 0.03, sixty red team cases, two successful attacks, dataset hashes that
matched nothing — every number written by hand, against datasets that did not exist. `conformance`
read them and reported **L3 Proven** for a project one minute old. That is the conformance theatre
this project exists to prevent, committed by the project, in the first artifact a new user is
handed.

Results now say `status: not_run` with null values, and the schema refuses both `measured` with
nulls and `not_run` with numbers. The four eval blockers judge only a result that claims to be
measured, so the gate blocks a false claim and the badge withholds credit: a fresh install is L2 and
`conformance` names the file standing between it and L3, while `check` still passes. `evals/run.py`
closes the other half — it hashes the datasets it read, runs 47 real cases through the agent's own
guardrails, and is the only thing that can write `measured`. Its `--dry-run` reports harness health
and refuses to print a verdict, because a metric computed from a simulated answer measures the
simulation.

The plans were also asking every agent for `citation_accuracy`, including two with no citations to
be accurate about. A metric that does not apply reports null forever and reads as assurance; the
runner now has a closed vocabulary and a plan naming something nothing computes fails loudly.

**Standard 16, seven controls, and a new control type.** `api` joins the closed vocabulary —
filing these under `tool` would have avoided a spec change and also meant a reader whose service
failed a check finds a type that does not name what failed. Two are blockers with no grace period,
which departs from "no control is ever born blocking" and is worth defending: both describe a
credential that is already public. The other five warn until content 1.2.

The seven ship as the `api.edge` pack, with `standards/16-service-and-edge.md` as the prose half
of each rule — `validate` enforces the correspondence in both directions. The standard is
framework-neutral; `adapters/fastapi.md` is the transport adapter that shows what the same rules
look like in a running service: where the caller becomes an identity, what must not be in the
request contract, and why the guardrails live in the agent core rather than in middleware.

The layout is declared as globs in `agentarch.yaml`, not assumed from directory names — a Spring
project and a FastAPI project are both right and neither should rename anything to be checkable.
`AA-DEP-019` reads the direction of the arrows, and says in its own doc comment what it cannot
prove: dynamic imports, imports built from strings, a request handed in by a DI container. A check
claiming to prove absence would be worse than one admitting its reach.

**`contracts/openapi.json` is generated**, from `interface.routes`, the way `.mcp.json` is generated
from the allowlist. The digest covers the interface rather than the rendered bytes, so rewording a
summary is not an interface change while adding a route — or removing auth from one — is.

**Two blueprints answer the two questions the four did not.** `chatbot-web` is the first with a
frontend: one static chat page served by the service itself — no Node toolchain, no CDN — where
the browser calls only `/v1`. `no_client_side_model_access` holds there because there is no
provider credential anywhere for it to hold, and the approval card renders in the chat, so the
whole human-in-the-loop loop is visible in the browser. `mcp-server` is the serving side of MCP:
tools advertised from the reviewed `.tool.yaml` specs rather than from decoration, the
descriptions hashed so a consumer's rug-pull tripwire has something to pin, and the irreversible
tool pausing for a human however it was reached — an MCP call is a transport, not an approver.

**The core fits its smallest budget again.** It had grown past 6144 bytes, so the copilot and
windsurf targets could not render at all: `sync` failed for anyone who enabled them, and CI never
noticed because it only checked the three large targets. The fix is the one the budget exists to
force — the full glossary demoted to `standards/00-index.md`, five load-bearing terms kept,
routing paths shortened. Every target now renders in both languages, and CI renders all of them
in a scratch copy, so the next overflow fails the build instead of the user.

**Fixed:** `start` told you to run `python app/agent.py`, a file that does not exist — the first
command a new user copy-pastes. The printed pytest command fails under pytest 9; it is
`python -m pytest` everywhere now, including the generated workflows. The README recommended
`npx` and `pipx` channels nothing is published to, and a CI action pinned to a `@v1` tag that
does not exist. `upgrade` announced the CLI's own version as the content's — the confusion
init's closing line already documents. `check --update-baseline` with no baseline recorded did
nothing, silently. `init --jurisdictions` accepted what `start` rejects. The command layer grew
its first tests. The three blueprints that shipped a `deploy/k8s.yaml` with no guide now have a
`deploy/README.md`.

**`start` stopped asking about you.** It asked who was accountable and offered a default from
`git config`, which put a work-provisioned identity into two manifests in a personal project and
then into a second tool's suggestion menu, because a wrong value written by one tool is read as fact
by the next. Three questions now, all about the software. It also stopped inferring the contact
address: personal data leaving the machine for a file that gets committed, which nobody asked for.
`owner.accountable` is an edit you make looking at the manifest.

**CI runs the blueprints.** Nothing executed them before — the Go tests check the payload's shape,
and a Python module importing a name that does not exist is shaped perfectly. Every variant installs,
runs its pytest suite (33 to 42 tests depending on the blueprint, none needing a credential) and runs
its eval harness. The conformance assertion used to demand L3 and pass, agreeing with the lie; it now
demands L2 and that the only thing missing is the evidence.

**Fixed:** `pydantic` 2.10 has no wheel for CPython 3.14, so pip fell back to building
pydantic-core from source and failed — a pin that does not install is not a pin, it is a trap.
`.gitignore` is now the one file a blueprint merges rather than replaces; refusing to install
because a project has one blocks nearly every real project, and overwriting it deletes rules
somebody depended on. `AA-DEP-019` lived in the command and not the library, so the test that
exists to prove the rule is enforced could not see it.

## v0.1.3 — 2026-07-29

`cli/0.1.3`. Spec and content unchanged.

**`agentarch start` — one command to begin with.** The first thing anybody typed used to be
`agentarch init --profile standard --jurisdictions BR`, which asks a newcomer to decide what a
profile is and why a jurisdiction matters before anything has happened: three concepts, none of
them the reason they came. `start` asks in ordinary language instead — is the code already
written, what are you building, where are your users, who gets paged — and derives the flags. It
never asks a question it can answer for itself: it reads your name from `git config`, notices
whether the directory already has code, and only raises the framework question when the starting
point ships more than one. Then it installs, syncs, writes the answers into the manifests, and
runs `validate` and `check` so you see the project pass before touching it. In an empty directory,
bare `agentarch` does the same.

Every answer also has a flag, so the composition is scriptable and therefore testable:
`start --new --blueprint rag-support --owner "…" --jurisdictions BR --yes`.

**`install.sh` — installing without a toolchain.** Until now the only working channels were
`go install`, which asks for Go, and the container, which asks for Docker. Both contradict the
reason the CLI is a single static binary in the first place: your project's language is its own
business. One line now does it:

```
curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/install.sh | sh
```

POSIX `sh`, no `jq`, no `sudo` on your behalf — without write access to `/usr/local/bin` it uses
`~/.local/bin` and says how to add it. It verifies the archive against the signed `checksums.txt`
and **refuses to install on a mismatch**, and it refuses just as firmly when neither `sha256sum`
nor `shasum` exists rather than skipping the check: an installer for a governance tool that
shrugs at an unverified download is arguing against itself. It also prints the `cosign` command,
and is honest that a checksum over HTTPS is not the same proof as a signature.

**`isTTY` counted `/dev/null` as a terminal.** It is a character device, and it is what a CI
runner hands a step for stdin — so an interactive prompt in CI read EOF and silently took the
default answer to every question instead of stopping to say there was nobody to ask. Found while
writing the assertions for `start`, and it affected `blueprint` too.

## v0.1.2 — 2026-07-29

`cli/0.1.2`. Spec and content unchanged.

Two fixes found by walking the getting-started path as a new user would.

**`go install` reported the wrong version.** The release build injects it with an ldflag, which
`go install` does not run, so anyone installing v0.1.1 was told they had a dev build with no way
to find out which. It now falls back to the module version Go records in the binary.

**The rag blueprint's own example question retrieved nothing.** The placeholder retriever
intersected exact words, so `order` did not match `Orders` and `shipping` did not match `ship`.
Following the README produced `[ESCALATE] no citation` on the first run — the worst first
impression for an agent whose whole argument is grounded answers. Prefix matching fixes it, and
`"what is your refund policy"` still retrieves nothing, which is the escalation path working.

CI now installs the dependencies and **imports** each blueprint rather than parsing it. A syntax
check never runs a decorator, a top-level statement or a dataclass, and all three have broken
here.

## v0.1.1 — 2026-07-29

`cli/0.1.1`. Spec and content unchanged.

The v0.1.0 release published fifteen signed artifacts and then failed to push the container: OCI
repository names must be lowercase and the owner is not. Rerunning did not help, because a
tag-triggered workflow reruns from the tag.

Fixed by computing the owner rather than hardcoding it, so a fork works too, and the image now
receives its version instead of reporting `dev`.

The image could have been pushed by hand from a laptop. It was not: an artifact built outside the
pipeline has no provenance and is not reproducible, which would undo the point of signing the
rest.

## v0.1.0 — 2026-07-29

First release. `spec/1.0.0-draft.1`, `content/1.0.0`, `cli/0.1.0`.

The spec is a draft on purpose: it has not been implemented twice, and calling it 1.0 before a
second implementation has exercised it would be claiming something nobody has tested.

### What it does

**Start something.** `agentarch blueprint` asks what you are building and installs a complete
project for it — manifest, prompt, tools, evals, threat model, CI, and code that runs. Four to
begin with: grounded RAG with citations, a tool-using agent with human approval, an orchestrator
with a typed handoff, and an MCP consumer with the rug-pull defence wired in. Each passes the
gate the moment it lands.

**Or adopt what exists.** `init --adopt` scans a project that already has agents and describes
what it finds, leaving everything it could not determine as `unknown`. `check --adopt-baseline`
records today's failures so the gate blocks only what is new or worse.

**Then keep it honest.** `validate` for structure, `check` for the release gate, `conformance`
for L1/L2/L3 with an expiry, `mcp audit --probe` for servers that changed since review, `diff`
for revalidation triggers, `report`, `score`, `aibom`.

Every assistant reads the same rules: `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `QWEN.md`, Cursor,
Copilot, Windsurf, generated from one core and verified in CI.

### Contents

16 standards · 39 controls · 9 packs including GDPR, LGPD and the EU AI Act · 11 framework
adapters · 5 skills with 5 matching checklists · `en` and `pt-BR` · 12 JSON Schemas · 8 normative
documents · 43 conformance fixtures.

### Two things that will not change

A pack is data, never code. The core is a fixed byte budget, not a list.

### Known limits, stated plainly

- The blueprints' application code has been syntax-checked, not executed against a live model.
- npm and PyPI publish once their tokens are configured; today use `go install` or the container.
- The community registry works and is empty.
- Nothing has been verified outside this repository yet. Everything was checked against material
  written here, which is the weakest form of evidence a standard can have.
