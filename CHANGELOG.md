# Changelog

Three version lines move independently — see `spec/normative/08-versioning.md`. Each entry says
which one changed.

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
LangGraph variant of rag-support differs from the no-framework one in exactly two files, which is
that claim demonstrated rather than asserted.

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

The layout is declared as globs in `agentarch.yaml`, not assumed from directory names — a Spring
project and a FastAPI project are both right and neither should rename anything to be checkable.
`AA-DEP-019` reads the direction of the arrows, and says in its own doc comment what it cannot
prove: dynamic imports, imports built from strings, a request handed in by a DI container. A check
claiming to prove absence would be worse than one admitting its reach.

**`contracts/openapi.json` is generated**, from `interface.routes`, the way `.mcp.json` is generated
from the allowlist. The digest covers the interface rather than the rendered bytes, so rewording a
summary is not an interface change while adding a route — or removing auth from one — is.

**`start` stopped asking about you.** It asked who was accountable and offered a default from
`git config`, which put a work-provisioned identity into two manifests in a personal project and
then into a second tool's suggestion menu, because a wrong value written by one tool is read as fact
by the next. Three questions now, all about the software. It also stopped inferring the contact
address: personal data leaving the machine for a file that gets committed, which nobody asked for.
`owner.accountable` is an edit you make looking at the manifest.

**CI runs the blueprints.** Nothing executed them before — the Go tests check the payload's shape,
and a Python module importing a name that does not exist is shaped perfectly. Every variant installs,
runs its pytest suite (33, 35, 34 and 42 tests, none needing a credential) and runs its eval harness.
The conformance assertion used to demand L3 and pass, agreeing with the lie; it now demands L2 and
that the only thing missing is the evidence.

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
