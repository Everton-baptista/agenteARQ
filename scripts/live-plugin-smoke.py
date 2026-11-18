#!/usr/bin/env python3
"""Real host integration smoke, not a code-quality benchmark. Python 3.11+, POSIX.

Effects: creates a controlled fixture and invokes an already authenticated host.
Only skill reads and two read-only AgenteARQ MCP calls are authorized. Never logs
credentials, prompts, completions, tool contents, shell commands, or raw transcripts.
"""
import argparse
import datetime
import hashlib
import json
import math
import os
from pathlib import Path
import re
import selectors
import shlex
import shutil
import signal
import subprocess
import time
import tomllib

READ_TOOLS = ("get_workflow", "get_next_task")
SKILL = "agentarch-discover-product"
SKILL_DIRS = {"codex": ".agents/skills", "claude": ".claude/skills", "gemini": ".gemini/skills"}
PROVIDERS = {"codex": "openai", "claude": "anthropic", "gemini": "google"}
MAX_BYTES = 8 * 1024 * 1024
NUMERIC_USAGE = {"input_tokens", "output_tokens", "cached_input_tokens", "cache_write_input_tokens",
                 "cache_read_input_tokens", "cache_creation_input_tokens", "reasoning_output_tokens",
                 "total_tokens", "cached", "input"}
IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}\Z")
SECRET = re.compile(r"(?:sk-[A-Za-z0-9_-]{16,}|AIza[A-Za-z0-9_-]{24,}|Bearer\s+\S{20,}|gh[pousr]_[A-Za-z0-9]{20,})", re.I)
CODEX_CLASSIFIER_VERSION = "1.0"
CODEX_AUXILIARY_ITEMS = frozenset({"agent_message", "reasoning", "todo_list"})
# These labels are retained only as metadata; membership does not authorize a tool.
CODEX_ITEM_LABELS = CODEX_AUXILIARY_ITEMS | frozenset({
    "mcp_tool_call", "command_execution", "file_change", "web_search", "error",
    "dynamic_tool_call", "collab_tool_call", "image_view", "image_generation",
    "context_compaction", "tool_search",
})
CODEX_EVENT_LABELS = frozenset({"thread.started", "turn.started", "turn.completed",
                               "turn.failed", "item.started", "item.updated", "item.completed", "error"})


class SmokeError(ValueError):
    """Contains only a fixed diagnostic code, never subprocess output."""


def require(condition, code):
    if not condition:
        raise SmokeError(code)


def identifier(value):
    return isinstance(value, str) and bool(IDENTIFIER.fullmatch(value)) and not SECRET.search(value)


def numbers(value):
    if not isinstance(value, dict):
        return {}
    return {key: number for key, number in value.items() if key in NUMERIC_USAGE
            and type(number) is int and 0 <= number < 10**12}


def error_result(value):
    if not isinstance(value, dict):
        return False
    return (any(key in value and value[key] is not False for key in ("isError", "is_error"))
            or ("error" in value and value["error"] is not None and value["error"] is not False))


def skill_path(project, host):
    return project / SKILL_DIRS[host] / SKILL / "SKILL.md"


def is_skill_path(value, target, project):
    if not isinstance(value, str):
        return False
    path = Path(value)
    return (path if path.is_absolute() else project / path).resolve() == target.resolve()


def command_reads_skill(command, target, project):
    """Accept an actual cat operation, not a command merely mentioning SKILL.md."""
    if not isinstance(command, str):
        return False
    try:
        parts = shlex.split(command)
        if len(parts) == 3 and Path(parts[0]).name in {"bash", "sh", "zsh"} and parts[1] in {"-lc", "-c"}:
            parts = shlex.split(parts[2])
    except ValueError:
        return False
    if len(parts) == 3 and parts[1] == "--":
        parts.pop(1)
    return len(parts) == 2 and parts[0] in {"cat", "/bin/cat", "/usr/bin/cat"} and is_skill_path(parts[1], target, project)


def classify_codex_events(raw, allowed_mcp_tools=READ_TOOLS, project=None):
    """Classify attempts without retaining IDs, commands, arguments, or item content.

    Only the caller's MCP allowlist and (when project is supplied) the exact skill
    read are authorized. Unknown metadata remains a blocker, never an auxiliary.
    Item IDs are used only in memory to count an attempt once across its lifecycle.
    """
    summary = {"classifier_version": CODEX_CLASSIFIER_VERSION, "event_counts": {},
               "started_item_counts": {}, "completed_item_counts": {},
               "unknown_type_hash_counts": {}, "auxiliary_items": 0,
               "prohibited_tool_attempts": 0, "unclassified_items": 0,
               "unclassified_events": 0, "incomplete_tool_items": 0,
               "invalid_item_lifecycle": 0, "host_error_items": 0, "host_error_events": 0,
               "invalid_event_lines": 0, "metadata_limit_exceeded": False}
    items = {}

    def count(field, label):
        summary[field][label] = summary[field].get(label, 0) + 1

    def label(value, known):
        if isinstance(value, str) and value in known:
            return value
        if isinstance(value, str):
            digest = hashlib.sha256(value.encode("utf-8", errors="surrogatepass")).hexdigest()
            if digest in summary["unknown_type_hash_counts"] or len(summary["unknown_type_hash_counts"]) < 32:
                count("unknown_type_hash_counts", digest)
            else:
                summary["metadata_limit_exceeded"] = True
        return "unknown"

    for index, line in enumerate(raw.splitlines()):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            summary["invalid_event_lines"] += 1
            continue
        if not isinstance(event, dict) or not isinstance(event.get("type"), str):
            summary["invalid_event_lines"] += 1
            continue
        kind = label(event["type"], CODEX_EVENT_LABELS)
        count("event_counts", kind)
        if kind == "unknown":
            summary["unclassified_events"] += 1
        if kind in {"error", "turn.failed"}:
            summary["host_error_events"] += 1
        if kind not in {"item.started", "item.updated", "item.completed"}:
            continue
        item = event.get("item")
        if not isinstance(item, dict) or not isinstance(item.get("type"), str):
            summary["invalid_item_lifecycle"] += 1
            continue
        item_type = label(item["type"], CODEX_ITEM_LABELS)
        if kind in {"item.started", "item.completed"}:
            count("started_item_counts" if kind == "item.started" else "completed_item_counts", item_type)
        if item_type in CODEX_AUXILIARY_ITEMS:
            if kind == "item.completed":
                summary["auxiliary_items"] += 1
            continue
        if item_type == "error":
            summary["host_error_items"] += 1
            continue
        allowed = ((item_type == "mcp_tool_call" and item.get("server") == "agentarch" and isinstance(item.get("tool"), str)
                    and item.get("tool") in allowed_mcp_tools)
                   or (item_type == "command_execution" and project is not None
                       and command_reads_skill(item.get("command"), skill_path(project, "codex"), project)))
        category = "allowed" if allowed else "unknown" if item_type == "unknown" else "prohibited"
        identity = (item_type, item.get("server"), item.get("tool"))
        call_id = item.get("id")
        if not isinstance(call_id, str) or not call_id:
            # Completed-only records are accepted by the native parser; an open
            # lifecycle without an ID cannot be correlated and fails closed.
            if kind != "item.completed":
                summary["invalid_item_lifecycle"] += 1
            call_id = ("anonymous", index)
        prior = items.get(call_id)
        if prior is None:
            items[call_id] = {"identity": identity, "category": category, "completed": False}
            if category != "allowed":
                summary["unclassified_items" if category == "unknown" else "prohibited_tool_attempts"] += 1
            prior = items[call_id]
        elif prior["identity"] != identity or prior["category"] != category or prior["completed"] or kind == "item.started":
            summary["invalid_item_lifecycle"] += 1
        if kind == "item.completed":
            prior["completed"] = True
    summary["incomplete_tool_items"] = sum(not item["completed"] for item in items.values())
    return summary


def codex_classification_passed(summary):
    return not any(summary[key] for key in (
        "prohibited_tool_attempts", "unclassified_items", "unclassified_events",
        "incomplete_tool_items", "invalid_item_lifecycle", "host_error_items", "host_error_events",
        "invalid_event_lines", "metadata_limit_exceeded"))


def new_trace():
    return {"terminal_success": False, "skill_load": "not_observed", "successful_mcp_reads": [],
            "failed_tool_calls": 0, "host_error_events": 0, "unexpected_tool_calls": 0,
            "usage": {}, "cost_usd": None, "observed_models": [], "mcp_discovery": "not_observed",
            "skill_discovery": "not_observed", "invalid_event_lines": 0}


def parse_trace(host, raw, project):
    """Only explicit native events prove execution; final prose is never evidence."""
    trace, pending, successful, models = new_trace(), {}, set(), set()
    if host == "codex":
        trace["codex_event_classification"] = classify_codex_events(raw, project=project)
    target = skill_path(project, host)

    def model(value):
        if identifier(value):
            models.add(value)

    def register(call_id, name, params):
        if not isinstance(call_id, str) or not call_id or call_id in pending:
            trace["host_error_events"] += 1
            return
        params = params if isinstance(params, dict) else {}
        prefixes = {"claude": "mcp__agentarch__", "gemini": "agentarch_"}
        prefix = prefixes.get(host, "")
        read = name[len(prefix):] if isinstance(name, str) and name.startswith(prefix) else None
        skill = ((host == "claude" and name == "Read" and is_skill_path(params.get("file_path"), target, project))
                 or (host == "gemini" and name == "activate_skill" and params.get("name") == SKILL))
        if read not in READ_TOOLS and not skill:
            trace["unexpected_tool_calls"] += 1
        pending[call_id] = {"read": read if read in READ_TOOLS else None, "skill": skill, "done": False}

    def finish(call_id, ok):
        item = pending.get(call_id) if isinstance(call_id, str) else None
        if not item or item["done"]:
            trace["host_error_events"] += 1
            return
        item["done"] = True
        if not ok:
            trace["failed_tool_calls"] += 1
        elif item["skill"]:
            trace["skill_load"] = "passed"
        elif item["read"]:
            successful.add(item["read"])

    for line in raw.splitlines():
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            trace["invalid_event_lines"] += 1
            continue
        if not isinstance(event, dict):
            trace["invalid_event_lines"] += 1
            continue
        kind = event.get("type")
        if not isinstance(kind, str):
            trace["invalid_event_lines"] += 1
            continue
        if kind in {"error", "turn.failed"}:
            trace["host_error_events"] += 1
        if host == "codex":
            if kind == "turn.completed":
                trace["terminal_success"] = True
                trace["usage"] = numbers(event.get("usage"))
            if kind == "item.completed":
                item = event.get("item", {})
                if not isinstance(item, dict):
                    trace["host_error_events"] += 1
                    continue
                if item.get("type") == "mcp_tool_call":
                    allowed = item.get("server") == "agentarch" and item.get("tool") in READ_TOOLS
                    if not allowed:
                        trace["unexpected_tool_calls"] += 1
                    elif item.get("status") == "completed" and not error_result(item) and not error_result(item.get("result")):
                        successful.add(item["tool"])
                    else:
                        trace["failed_tool_calls"] += 1
                elif item.get("type") == "command_execution":
                    if command_reads_skill(item.get("command"), target, project):
                        if item.get("exit_code") == 0 and item.get("status") in (None, "completed"):
                            trace["skill_load"] = "passed"
                        else:
                            trace["failed_tool_calls"] += 1
                    else:
                        trace["unexpected_tool_calls"] += 1
                elif item.get("type") not in ("agent_message", "reasoning", "todo_list"):
                    trace["unexpected_tool_calls"] += 1
        elif host == "claude":
            if kind == "system" and event.get("subtype") == "init":
                servers = event.get("mcp_servers", [])
                for server in servers if isinstance(servers, list) else []:
                    if isinstance(server, dict) and server.get("name") == "agentarch" and server.get("status") == "connected":
                        trace["mcp_discovery"] = "connected"
                if isinstance(event.get("skills"), list) and SKILL in event["skills"]:
                    trace["skill_discovery"] = "discovered"
            message = event.get("message", {})
            if isinstance(message, dict):
                if kind == "assistant":
                    model(message.get("model"))
                content = message.get("content", [])
                for block in content if isinstance(content, list) else []:
                    if not isinstance(block, dict):
                        continue
                    if kind == "assistant" and block.get("type") == "tool_use":
                        register(block.get("id"), block.get("name"), block.get("input"))
                    if kind == "user" and block.get("type") == "tool_result":
                        finish(block.get("tool_use_id"), not error_result(block))
            if kind == "result":
                trace["terminal_success"] = event.get("subtype") == "success" and event.get("is_error") is False
                if not trace["terminal_success"]:
                    trace["host_error_events"] += 1
                trace["usage"] = numbers(event.get("usage"))
                cost = event.get("total_cost_usd")
                if type(cost) in {int, float} and math.isfinite(cost) and cost >= 0:
                    trace["cost_usd"] = cost
        else:
            if kind == "init":
                model(event.get("model"))
            if kind == "tool_use":
                register(event.get("tool_id"), event.get("tool_name"), event.get("parameters"))
            if kind == "tool_result":
                finish(event.get("tool_id"), event.get("status") == "success" and not error_result(event))
            if kind == "result":
                trace["terminal_success"] = event.get("status") == "success" and not error_result(event)
                if not trace["terminal_success"]:
                    trace["host_error_events"] += 1
                stats = event.get("stats", {})
                trace["usage"] = numbers(stats)
                if isinstance(stats, dict) and isinstance(stats.get("models"), dict):
                    for name in stats["models"]:
                        model(name)
    if any(not item["done"] for item in pending.values()):
        trace["host_error_events"] += 1
    trace["successful_mcp_reads"], trace["observed_models"] = sorted(successful), sorted(models)
    return trace


def trace_passed(trace, process, unchanged=True):
    return (process["exit_code"] == 0 and not process["timed_out"] and not process["output_limit"]
            and trace["terminal_success"] and trace["skill_load"] == "passed"
            and set(trace["successful_mcp_reads"]) == set(READ_TOOLS)
            and not any(trace[key] for key in ("failed_tool_calls", "host_error_events", "unexpected_tool_calls", "invalid_event_lines"))
            and ("codex_event_classification" not in trace or codex_classification_passed(trace["codex_event_classification"]))
            and trace["usage"].get("output_tokens", 0) > 0 and unchanged)


def terminate(process):
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        process.wait(timeout=2)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=2)


def run(argv, cwd, env=None, timeout=30):
    """Bounded in-memory capture. No raw stream is written, even on errors."""
    started = time.monotonic()
    process = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    streams = selectors.DefaultSelector()
    buffers = {"stdout": bytearray(), "stderr": bytearray()}
    for name in buffers:
        streams.register(getattr(process, name), selectors.EVENT_READ, name)
    timed_out = output_limit = False
    try:
        while streams.get_map():
            if time.monotonic() - started >= timeout:
                timed_out = True
                break
            for key, _ in streams.select(timeout=min(.1, max(0, timeout - (time.monotonic() - started)))):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    streams.unregister(key.fileobj)
                    continue
                if sum(len(value) for value in buffers.values()) + len(chunk) > MAX_BYTES:
                    output_limit = True
                    break
                buffers[key.data].extend(chunk)
            if output_limit:
                break
        if timed_out or output_limit:
            terminate(process)
        else:
            try:
                process.wait(timeout=max(.01, timeout - (time.monotonic() - started)))
            except subprocess.TimeoutExpired:
                timed_out = True
                terminate(process)
    finally:
        terminate(process)  # Also clean up children still holding inherited pipe descriptors.
        streams.close()
        process.stdout.close()
        process.stderr.close()
    metadata = {"exit_code": process.returncode, "timed_out": timed_out, "output_limit": output_limit,
                "duration_ms": round((time.monotonic() - started) * 1000),
                **{name + "_sha256": hashlib.sha256(data).hexdigest() for name, data in buffers.items()}}
    return metadata, *(bytes(buffers[name]).decode("utf-8", errors="replace") for name in ("stdout", "stderr"))


def auth_status(host, binary, cwd):
    if host == "gemini":
        home = Path(os.environ.get("GEMINI_CLI_HOME", str(Path.home())))
        presence = {key: bool(os.environ.get(key)) for key in ("GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS")}
        for name in ("oauth_creds.json", "google_accounts.json"):
            presence[name] = (home / ".gemini" / name).is_file()
        presence["adc"] = (Path.home() / ".config/gcloud/application_default_credentials.json").is_file()
        return {"status": "credential_present_unverified" if any(presence.values()) else "missing",
                "method": "presence_only", "presence": presence}
    args = [binary, "login", "status"] if host == "codex" else [binary, "auth", "status", "--json"]
    process, out, err = run(args, cwd)
    ready = False
    if not process["timed_out"] and not process["output_limit"]:
        if host == "codex":
            ready = process["exit_code"] == 0 and "logged in" in (out + err).lower()
        else:
            try:
                ready = process["exit_code"] == 0 and json.loads(out).get("loggedIn") is True
            except (ValueError, AttributeError):
                pass
    return {"status": "authenticated" if ready else "missing_or_unavailable", "method": "native_status", "process": process}


def managed_transport(project, host, binary):
    if host == "codex":
        server = tomllib.loads((project / ".codex/config.toml").read_text())["mcp_servers"]["agentarch"]
    else:
        path = project / (".mcp.json" if host == "claude" else ".gemini/settings.json")
        server = json.loads(path.read_text())["mcpServers"]["agentarch"]
    # Claude's .mcp.json is committed and shared, so its root is "." and the server resolves the
    # project upward from its launch directory; the other hosts receive the absolute root.
    root = "." if host == "claude" else str(project)
    require(server.get("command") == binary and server.get("args") == ["mcp", "serve", "--root", root], "managed_transport_mismatch")
    return server


def codex_overrides(project, server):
    overrides = [f'projects.{json.dumps(str(project))}.trust_level="trusted"',
                 'model_reasoning_effort="medium"',
                 "mcp_servers.agentarch.command=" + json.dumps(server["command"]),
                 "mcp_servers.agentarch.args=" + json.dumps(server["args"]),
                 "mcp_servers.agentarch.enabled_tools=" + json.dumps(list(READ_TOOLS)),
                 "mcp_servers.agentarch.required=true", 'mcp_servers.agentarch.default_tools_approval_mode="approve"',
                 'features.apps=false', 'features.multi_agent=false', 'web_search="disabled"',
                 'shell_environment_policy.inherit="core"',
                 'shell_environment_policy.exclude=["*KEY*","*TOKEN*","*SECRET*","*PASSWORD*"]']
    cache = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))) / "plugins/cache"
    if cache.is_dir():
        for market in sorted(cache.iterdir()):
            if market.is_dir():
                for plugin in sorted(market.iterdir()):
                    if plugin.is_dir():
                        overrides.append(f'plugins.{json.dumps(plugin.name + "@" + market.name)}.enabled=false')
    return [arg for value in overrides for arg in ("-c", value)]


def host_command(host, binary, model, project, server):
    target = skill_path(project, host)
    prompt = (f"Perform only this authorized read-only integration inspection. Load the installed {SKILL} skill "
              f"through the host (read {target}; Codex should use cat). Gemini should use activate_skill. "
              "Treat this as inspection, not permission to execute the skill workflow or create artifacts. "
              "Call the AgenteARQ MCP get_workflow and get_next_task tools once each. "
              "Report the observed workflow phase. Do not edit, run unrelated commands, call other tools, "
              "or fabricate results. If authentication or approval blocks a call, report it and stop.")
    if host == "codex":
        return [binary, "--no-daemon", "-a", "never", "exec", "--ignore-user-config", "--ephemeral",
                "--sandbox", "read-only", "--model", model, "--cd", str(project), "--json", "--color", "never",
                *codex_overrides(project, server), prompt]
    if host == "claude":
        read = f"Read(./{SKILL_DIRS[host]}/{SKILL}/SKILL.md)"
        return [binary, "--strict-mcp-config", "--mcp-config", str(project / ".mcp.json"),
                "--setting-sources", "project", "--settings", '{"disableAllHooks":true,"disableClaudeAiConnectors":true}',
                "--tools", "Read,Skill", "--allowedTools", ",".join([read, *("mcp__agentarch__" + tool for tool in READ_TOOLS)]),
                "--disallowedTools", "mcp__agentarch__record_checkpoint,mcp__agentarch__verify_project,mcp__agentarch__inspect_project",
                "--permission-mode", "dontAsk", "--no-chrome", "--no-session-persistence", "--model", model,
                "--effort", "low", "--max-turns", "6", "--max-budget-usd", "0.50", "--output-format", "stream-json", "--verbose", "-p", prompt]
    return [binary, "--model", model, "--approval-mode", "plan", "--allowed-mcp-server-names", "agentarch",
            "--output-format", "stream-json", "--prompt", prompt]


def source_fingerprint(project):
    digest = hashlib.sha256()
    for path in sorted(project.rglob("*")):
        rel = path.relative_to(project)
        if ".git" in rel.parts and rel.as_posix() != ".git/agentarch-workflow/state.json":
            continue
        if path.is_symlink():
            digest.update(str(rel).encode() + b"\0symlink\0" + os.readlink(path).encode())
        elif path.is_file():
            digest.update(str(rel).encode() + b"\0" + path.read_bytes())
    return digest.hexdigest()


def setup_fixture(args, output, report):
    project = output / "project"
    project.mkdir()
    commands = [["git", "init", "-q", str(project)],
                [args.agentarch_bin, "init", "--root", str(project), "--profile", "standard", "--lang", "en"],
                [args.agentarch_bin, "workflow", "init", "--root", str(project), "--name", "Host integration fixture",
                 "--profile", "universal", "--goal", "Observe skill loading and two read MCP calls",
                 "--author", args.owner, "--executors", "agentarch-" + args.host, "--format", "json"],
                [args.agentarch_bin, "plugin", "install", args.host, "--root", str(project), "--owner", args.owner,
                 "--contact", args.contact, "--model-provider", PROVIDERS[args.host], "--model", args.model,
                 "--binary", args.agentarch_bin, "--format", "json"]]
    for index, command in enumerate(commands):
        metadata, _, _ = run(command, project)
        report.setdefault("setup", []).append({"step": index, **metadata})
        require(metadata["exit_code"] == 0 and not metadata["timed_out"] and not metadata["output_limit"], "fixture_setup_failed")
    server = managed_transport(project, args.host, args.agentarch_bin)
    env = os.environ.copy()
    if args.host == "gemini":
        trust = output / "trusted-folders.json"
        trust.write_text(json.dumps({str(project): "TRUST_FOLDER"}) + "\n")
        trust.chmod(0o600)
        env["GEMINI_CLI_TRUSTED_FOLDERS_PATH"] = str(trust)
        settings = project / ".gemini/settings.json"
        value = json.loads(settings.read_text())
        value.update({"tools": {"core": ["activate_skill"], "exclude": ["agentarch_record_checkpoint", "agentarch_verify_project", "agentarch_inspect_project"]},
                      "model": {"maxSessionTurns": 6}, "billing": {"overageStrategy": "never"},
                      "hooksConfig": {"enabled": False},
                      "telemetry": {"enabled": False, "logPrompts": False}})
        settings.write_text(json.dumps(value, indent=2) + "\n")
    return project, server, env


def discover(args, project, server, env):
    result = {"mcp": "not_observed", "skills": "installed_only"}
    if args.host == "gemini":
        metadata, out, err = run([args.host_binary, "mcp", "list"], project, env)
        result["process"] = metadata
        if metadata["exit_code"] == 0 and re.search(r"agentarch:.*\bConnected\b", out + err):
            result["mcp"] = "connected"
        _, out, err = run([args.host_binary, "skills", "list"], project, env)
        if re.search(r"^" + SKILL + r" \[Enabled\]", out + err, re.M):
            result["skills"] = "discovered"
    elif args.host == "codex":
        metadata, out, _ = run([args.host_binary, "mcp", "list", "--json", *codex_overrides(project, server)], project, env)
        result["process"] = metadata
        try:
            if metadata["exit_code"] == 0 and any(row.get("name") == "agentarch" for row in json.loads(out) if isinstance(row, dict)):
                result["mcp"] = "configured"  # Configuration listing is not a successful connection.
        except (ValueError, TypeError):
            pass
    # Claude's native init stream supplies authoritative discovery during model execution.
    return result


def write_report(output, report):
    temp = output / "report.json.tmp"
    temp.write_text(json.dumps(report, indent=2, allow_nan=False) + "\n")
    temp.replace(output / "report.json")


def executable(value):
    path = shutil.which(value)
    require(path is not None and os.access(path, os.X_OK), "executable_missing")
    return str(Path(path).absolute())


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", choices=tuple(PROVIDERS), required=True)
    for name in ("host-binary", "model", "owner", "contact", "agentarch-bin", "output-dir"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--preflight-only", action="store_true", help="Install/discover/check auth without model inference")
    parser.add_argument("--timeout", type=int, default=180, help="Model timeout in seconds, 1..180")
    args = parser.parse_args(argv)
    report = {"schema_version": "1.0", "host": args.host, "status": "blocked", "reason": "not_started",
              "model_requested": args.model, "model_attempted": False, "capture_content": False,
              "at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "bounds": {"model_timeout_seconds": args.timeout, "allowed_mcp_tools": list(READ_TOOLS),
                         "host_policy": {"codex": "read-only sandbox, never request approval", "claude": "Read/Skill tools, dontAsk, explicit read allowlist", "gemini": "plan, activate_skill builtin, MCP writes excluded"}[args.host],
                         "harness_modifies_global_config": False},
              "discovery": {"mcp": "not_observed", "skills": "not_observed"}, "model_trace": new_trace()}
    output = None
    try:
        require(os.name == "posix", "posix_required")
        require(identifier(args.model) and args.model not in {"auto", "latest", "sonnet", "opus", "haiku"} and not args.model.endswith("-latest"), "explicit_model_id_required")
        require(1 <= args.timeout <= 180, "timeout_out_of_bounds")
        require(all(0 < len(value.strip()) <= 200 and not SECRET.search(value) for value in (args.owner, args.contact)), "invalid_identity")
        path = Path(args.output_dir).absolute()
        require(not any(parent.is_symlink() for parent in (path, *path.parents)), "symlink_output_rejected")
        require(not any((parent / ".git").exists() or (parent / "AGENTS.md").exists() for parent in (path, *path.parents)), "output_must_be_outside_repository")
        require(not path.exists() or not any(path.iterdir()), "output_directory_not_empty")
        path.mkdir(parents=True, exist_ok=True)
        output = path.resolve()
        args.host_binary, args.agentarch_bin = executable(args.host_binary), executable(args.agentarch_bin)
        report["agentarch_sha256"] = hashlib.sha256(Path(args.agentarch_bin).read_bytes()).hexdigest()
        version_meta, version, _ = run([args.host_binary, "--version"], output)
        found = re.search(r"\b\d+\.\d+\.\d+\b", version)
        report["host_version"] = found.group() if found else None
        require(version_meta["exit_code"] == 0 and found is not None, "host_version_unavailable")
        project, server, env = setup_fixture(args, output, report)
        report["installed_skill_sha256"] = hashlib.sha256(skill_path(project, args.host).read_bytes()).hexdigest()
        report["discovery"] = discover(args, project, server, env)
        for name, command in (("plugin_check", ["plugin", "check", args.host]), ("validate", ["validate"]), ("policy_gate", ["check"])):
            metadata, _, _ = run([args.agentarch_bin, *command, "--root", str(project)], project)
            report[name] = metadata
        require(report["plugin_check"]["exit_code"] == 0 and report["validate"]["exit_code"] == 0, "plugin_artifact_check_failed")
        # The standard policy gate remains independent; plugin discovery cannot certify host guardrails.
        report["authentication"] = auth_status(args.host, args.host_binary, project)
        if report["authentication"]["status"] not in {"authenticated", "credential_present_unverified"}:
            report["reason"] = "authentication_missing_or_unavailable"
        elif args.preflight_only:
            report.update(status="preflight_only", reason="model_not_requested")
        else:
            before = source_fingerprint(project)
            report.update(model_attempted=True, reason="model_started")
            write_report(output, report)
            metadata, out, _ = run(host_command(args.host, args.host_binary, args.model, project, server), project, env, args.timeout)
            trace = parse_trace(args.host, out, project)
            report["model_process"], report["model_trace"] = metadata, trace
            report["source_unchanged"] = source_fingerprint(project) == before
            for key, field in (("mcp", "mcp_discovery"), ("skills", "skill_discovery")):
                if trace[field] != "not_observed":
                    report["discovery"][key] = trace[field]
            passed = trace_passed(trace, metadata, report["source_unchanged"])
            report.update(status="passed" if passed else "failed_or_incomplete", reason="observed_skill_and_two_reads" if passed else "required_evidence_missing_or_failed")
    except (SmokeError, OSError, ValueError, KeyError, TypeError) as error:
        report.update(status="blocked", reason=str(error) if isinstance(error, SmokeError) else "local_configuration_or_process_error")
    if output:
        write_report(output, report)
    print(json.dumps({"host": args.host, "status": report["status"], "reason": report["reason"],
                      "report": str(output / "report.json") if output else None}))
    return 0 if report["status"] in {"passed", "preflight_only"} else 4


if __name__ == "__main__":
    raise SystemExit(main())
