# Changelog

Three version lines move independently — see `spec/normative/08-versioning.md`. Each entry says
which one changed.

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
