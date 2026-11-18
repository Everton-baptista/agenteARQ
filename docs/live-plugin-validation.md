# Real host plugin validation

`scripts/live-plugin-smoke.py` tests the installed AgenteARQ skills and MCP transport in
Codex CLI, Claude Code, or Gemini CLI. It is a bounded integration smoke, not the product
benchmark or a measurement of generated-code quality. Python 3.11+, Git, a POSIX system,
an AgenteARQ binary, and an already installed host CLI are required.

The script creates a fresh synthetic Git project outside existing repositories. It installs
the plugin with the supplied accountable person, contact, provider and model, validates the
artifacts, checks the plugin receipt, and records the independent standard policy gate.
It never installs a host, logs in, merges, deploys, or changes global configuration.

## Run

All seven identity/location arguments are required. Use the actual binary paths and an
explicit model identifier available to the account. The harness does not select a model,
silently fall back, or certify that a version-specific identifier is an immutable snapshot.
The output directory must be new or empty and outside any repository or ancestor `AGENTS.md`.

```sh
python3 scripts/live-plugin-smoke.py \
  --host codex --host-binary /absolute/path/to/codex \
  --model gpt-6-astra \
  --owner "Your name" --contact "Your project contact" \
  --agentarch-bin /absolute/path/to/agentarch \
  --output-dir /tmp/agentarch-codex-preflight \
  --preflight-only
```

Use `--host claude` or `--host gemini` with that host's binary and explicit model ID.
After reviewing the preflight, run the same command with a **new output directory** and
without `--preflight-only` to authorize the model smoke. It may consume the account's usage.
The default model timeout is 180 seconds; `--timeout` can lower it. The script kills the
process group on timeout and limits transient stdout/stderr capture to 8 MiB in memory.

Missing authentication produces `blocked` before inference. Codex and Claude use their
native authentication status command. Gemini has no equivalent status command in the
tested CLI, so the harness checks only environment/file-presence booleans, without reading
credential values. `credential_present_unverified` is not proof that a credential works;
expired or invalid authentication still prevents a successful model smoke. Authenticate
through the host's normal flow and run a new fixture when ready.

## What passes

A `passed` report requires all of these observed in the native host event stream:

1. The host loads `agentarch-discover-product`: Codex reads its exact installed `SKILL.md`
   with `cat`; Claude uses `Read` on that file; Gemini completes `activate_skill` for it.
2. Both `get_workflow` and `get_next_task` complete successfully through AgenteARQ MCP.
3. The terminal event succeeds, the process exits zero, output token usage is reported,
   no tool/error/incomplete event contradicts success, and the fixture remains unchanged.

For Claude and Gemini, tool results must correlate with an earlier tool call ID. A declared
tool or a model's claim in final prose is insufficient. Codex `status: completed` with an
`error` or `result.isError` fails. Claude `subtype: success` with `is_error: true` fails.
Unexpected tools, missing results, malformed event lines, or incomplete evidence fail closed.

Skill loading here demonstrates host access to installed skill content; it does not prove
that the model correctly follows the entire skill or chooses it automatically on other tasks.

The report separates `discovery` from `model_trace`. A Codex MCP configuration listing is
`configured`, not `connected`. Gemini can discover skills and connect to MCP before model
authentication. Claude's native session initialization provides discovery when a session
runs. None of those states alone passes the model smoke.

## Host boundaries

| Host | Temporary invocation boundary |
|---|---|
| Codex | `--sandbox read-only`, approval `never`, ephemeral session, explicit read-only MCP tool list and transport copied from the generated configuration |
| Claude | `Read,Skill` built-ins; explicit skill-file/read-MCP allowlist; `dontAsk`; strict MCP configuration; hooks/connectors disabled; no session persistence; six turns and USD 0.50 ceiling |
| Gemini | Plan mode, only `activate_skill` built-in, MCP writes excluded, only the AgenteARQ MCP server, hooks and telemetry disabled in fixture settings, six turns, no credit overages |

Codex CLI 0.160.1 did not reliably load the generated project transport solely from a
command-line trust override during the manual smoke. The harness verifies the generated
`command`/`args` and supplies them explicitly through `-c`. It ignores user configuration
for this isolated invocation and disables unrelated installed plugins, but **does not use
`--ignore-rules`**: host execution rules remain in force. Auth stays in the existing location.

Gemini uses the supported `GEMINI_CLI_TRUSTED_FOLDERS_PATH` variable pointing to a private
temporary file with exactly the controlled fixture root marked `TRUST_FOLDER`. It never
uses `--skip-trust`, trusts a parent, or rewrites the user's trust file. The harness does
not assign `HOME`, `CODEX_HOME`, or a replacement authentication directory for any host.

Claude and Gemini use host tool restrictions, not a claim of a new OS sandbox. Gemini may
retain synthetic session data according to its native settings; the harness does not copy
or export those host-owned sessions. Host-managed policy can still block the requested
read operations. A failed approval is reported, not worked around with bypass flags.

## Evidence and limitations

Only `report.json` and the synthetic fixture/configuration are retained. The report allows
host/version/model identifiers, executable/skill/output hashes, durations, exit codes,
boolean authentication metadata, discovery states, successful tool names, numeric usage,
and an actual cost if the host reports one. It omits account identity, credential values,
session IDs, model prose, tool content, generated shell commands, and raw transcripts.
Unknown cost is `null`; the script does not infer a price from token counts.

The Codex classifier records bounded type/count metadata for started and completed items.
Only messages, reasoning, and plan updates are auxiliary; unknown types remain blockers
and are represented by hashes, not arbitrary stream strings. Prohibited tool attempts also
block when they start without completing. IDs, commands, arguments, and item content are
not retained. The classifier can be reused with a separate explicit MCP allowlist by a
synthetic checkpoint probe; the public smoke command remains read-only.

The standard policy gate has a separate result. The plugin deliberately does not claim to
enforce the host's input/output guardrails, so those findings can block `agentarch check`
even when the integration smoke passes. A smoke `passed` is not release approval or L3
behavioral conformance.

Exit codes: `0` means the model smoke passed or the explicitly requested preflight finished;
`4` means blocked, failed, or incomplete; argparse uses `2` for invalid CLI arguments.
Read `status` to distinguish `preflight_only` from `passed`.

Parser/process tests make no provider calls:

```sh
python3 -m unittest discover -s scripts/tests -p test_live_plugin_smoke.py -v
```

The implementation was checked against local CLI help/source for Codex 0.160.1, Claude
Code 2.1.252, and Gemini CLI 0.68.0. New event formats require explicit parser review;
missing evidence must not become a pass. Manual runs of earlier harnesses are separate
evidence and are never imported as successful runs of this script.

Recorded run on 2026-11-16 using this script:

| Host | Observed result |
|---|---|
| Codex 0.160.1, requested `gpt-6-astra` | Real smoke passed: installed skill read, both MCP reads succeeded, no unexpected calls, source unchanged; 15.075 seconds |
| Claude Code 2.1.252 | Preflight blocked by missing authentication; no inference attempted |
| Gemini CLI 0.68.0 | Skills discovered and MCP connected; preflight blocked by missing authentication; no inference attempted |

The [durable summary](../benchmarks/results/2026-11-16/plugins/summary.json) links each
allowlisted host report. The Codex stream reports token usage but not an authoritative
backend snapshot identity or cost; those are not inferred. The separate standard policy
gate exited 4 for all three fixtures, retaining the declared host guardrail gaps.

A separate [Codex checkpoint proof](../benchmarks/results/2026-11-16/plugins-checkpoint-classifier-1.0/codex.json)
passed on the same date using classifier 1.0 and a newly built AgenteARQ binary. It observed
`get_workflow` at revision 1, one authorized `record_checkpoint`, and `get_workflow` at
revision 2; only the local state file changed. Its three MCP calls and two auxiliary messages
were classified with no unknown or prohibited items. This is a new execution: the earlier
[checkpoint report's two unclassified items](../benchmarks/results/2026-11-16/plugins-checkpoint-1.0.1/conclusion.json)
remain unknown, and its recorded outcome is preserved.

Official references: [Codex non-interactive mode](https://developers.openai.com/codex/noninteractive),
[Codex MCP](https://developers.openai.com/codex/mcp),
[Claude programmatic execution](https://code.claude.com/docs/en/headless),
[Gemini skills](https://geminicli.com/docs/cli/skills/),
[Gemini MCP](https://geminicli.com/docs/tools/mcp-server/),
[Gemini trusted folders](https://geminicli.com/docs/cli/trusted-folders/),
[Gemini authentication](https://geminicli.com/docs/get-started/authentication/).
