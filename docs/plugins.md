# Host integrations and portable skills

AgenteARQ installs its reviewed workflows into Codex, Claude Code, or Gemini CLI while
preserving the host's own permission model. The `external` target produces a reference
package for another client supporting Agent Skills and MCP stdio. Installation does not
publish a marketplace package, start an agent, authenticate a provider, or approve a workspace.

| Target | Skills | MCP configuration |
|---|---|---|
| `codex` | `.agents/skills/agentarch-*` | managed block in `.codex/config.toml` |
| `claude` | `.claude/skills/agentarch-*` | `mcpServers.agentarch` in `.mcp.json` |
| `gemini` | `.gemini/skills/agentarch-*` | `mcpServers.agentarch` in `.gemini/settings.json` |
| `external` | `agentarch/integrations/external/skills/` | reference `agentarch/integrations/external/mcp.json` |

The source is `content/skills/`, embedded in the released binary. Host copies are install
outputs, not separate skill libraries. `content/plugins/<host>.json` declares the destination,
capabilities, limitations, and official documentation. The installer downloads no skills.

The Python helpers are called through `python3`, as their skills document. They ship as
ordinary `0644` files without shebangs; installation does not grant executable permissions.
When testing helpers directly in `content/`, prevent Python bytecode writes (for example,
`PYTHONDONTWRITEBYTECODE=1`). `go test ./...` rejects `.pyc` files and `__pycache__` in the
embedded payload while checking that required blueprint dotfiles remain present. Run that
validation before packaging; `go build` alone does not perform the cleanliness check.

## Install, inspect, and remove

Choose an accountable person and contact, and the exact provider/model used by this deployment.
The installer requires these facts instead of guessing them from a Git account. Supplying
the owner also identifies the reviewer accepting this generated integration contract; review
the dry run and the source before installing under another person's name.

```sh
agentarch plugin install codex --dry-run \
  --owner "Your name" --contact "you@example.test" \
  --model-provider "your-provider" --model "exact-model-version"
agentarch plugin install codex \
  --owner "Your name" --contact "you@example.test" \
  --model-provider "your-provider" --model "exact-model-version"
agentarch plugin check codex
agentarch plugin remove codex --dry-run
agentarch plugin remove codex
```

Use the project root option supported by the CLI when running elsewhere. The Go API is
`Install(root, host, Options)`, `Check(root, host)`, and `Remove(root, host, Options)`.
`Options.Binary` can name an absolute `agentarch` executable. Generated MCP arguments are
an array containing `mcp`, `serve`, `--root`, and the canonical root; no shell interpolation
is involved. `ServerVersion` pins the MCP contract version, independently of a development
CLI build. The installer does not verify a binary's provenance or the provider's model ID.

Dry runs report file actions and backup paths without writing. Installs are idempotent.
Existing unrelated JSON values and TOML text survive. A different, preexisting AgenteARQ
server entry or skill is a conflict, not an invitation to overwrite it. Files that already
match the shipped skill are reused and retained when uninstalling.

The private receipt and backups live under `.agentarch/plugin-state/`, excluded by a local
`.gitignore`, with private file permissions. Backups can contain the host configuration's
existing credentials, so they stay local and are never printed or uploaded. Removing an
integration retains backups for recovery. Changed managed files must be restored or reviewed
before replacement/removal; unrelated settings added after installation remain intact.
Symlinked destinations are rejected. A leftover `install.lock` after a crash must be inspected
before another installer runs. Individual writes are atomic and ordinary write failures roll
back completed edits; killing the process mid-transaction can still require recovery from
the receipt and backups.

## What the contract covers

Each host gets a deployment manifest, a versioned prompt with its SHA-256, five tool contracts,
and a deny-by-default MCP allowlist with exact tool names and description hashes. These are
under `agentarch/project/agents/`, `tools/`, and `plugins/<host>/`. JSON syntax in `.yaml` files
is intentional: it is valid YAML and safely quotes user-supplied deployment facts.

The installation declares intended owner, model, prompt, and execution bounds. It does not
replace the host's system prompt or force its model selection. Use these artifacts when
configuring the executor and validate the effective host setup before claiming deployment
conformance. The manifest explicitly leaves additional input/output guardrails undeclared
and records that the host owns those boundaries. `check` verifies local receipt consistency;
it does not certify authentication, host discovery, effective sandboxing, or live behavior.

| MCP tool | Effect | Boundary |
|---|---|---|
| `inspect_project` | read | Current configured project |
| `get_workflow` | read | Current project's workflow |
| `get_next_task` | read | Eligible task and prerequisites |
| `record_checkpoint` | write | Local workflow progress, with expected revision |
| `verify_project` | write | Verification evidence, with expected revision |

`git-dir/agentarch-workflow/**` in tool contracts denotes state beneath the worktree's resolved
Git directory, not a user-supplied filesystem path. Verification uses the coordinator's
isolated verification runner and its configured timeout; the tool contract allows 30 minutes
plus startup/cleanup overhead. Checkpoints cannot certify independent review or publish a PR.
Both write operations also export `agentarch/project/evidence/latest.json`. They use optimistic
concurrency through `expected_revision`; replaying a successful write fails as stale rather
than returning a deduplicated success. Their contracts therefore declare no idempotency and
disable automatic retries. Read the workflow again before deciding on another write.

The MCP server only governs its own operations. Installing it cannot intercept another host's
shell command or supply an OS sandbox. Native inclusion settings narrow this server's tools
where supported; they do not change global approvals. Follow the host's workspace trust and
MCP approval prompts yourself. Never turn on bypass/YOLO as an installation workaround.

## Extending a host integration

The neutral boundary is Agent Skills plus MCP stdio. Add a reviewed descriptor and a tested
configuration writer only when the host's documented format is known. Preserve unknown
configuration and disclose unsupported capabilities. Test discovery separately from static
installation, and test actual cancellation, resumption, approval, and sandbox behavior for
each execution adapter/version. Missing capabilities must not silently become greater authority.

ACP can be added as an optional execution transport after capability negotiation. This plugin
installer does not claim native ACP support for Codex or Claude Code. Gemini documents ACP,
but ACP support alone does not guarantee equivalent sandboxing or cancellation of subprocesses.

Official references, checked 2026-11-16: [Agent Skills](https://agentskills.io/specification),
[Codex MCP](https://developers.openai.com/codex/mcp/),
[Codex skills](https://developers.openai.com/codex/skills/),
[Claude skills](https://code.claude.com/docs/en/skills),
[Claude MCP](https://code.claude.com/docs/en/mcp),
[Gemini skills](https://geminicli.com/docs/cli/skills/),
[Gemini MCP](https://geminicli.com/docs/tools/mcp-server/),
[ACP capabilities](https://agentclientprotocol.com/protocol/v1/initialization).
