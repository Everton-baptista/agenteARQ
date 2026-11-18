#!/usr/bin/env python3
"""Controlled product benchmark. Dry-run by default; never selects or invokes an AI host."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import random
import re
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from html.parser import HTMLParser
from pathlib import Path

VERSION = "1.0"
SUITE_PATH = Path(__file__).resolve().parents[1] / "benchmarks/products.json"
CONDITIONS = ["executor_only", "agentarch"]
EXPORT_PATH = "agentarch/project/evidence/latest.json"
HOST_ERROR_CATEGORIES = {
    "quota_exhausted",
    "rate_limited",
    "authentication_required",
    "network_unavailable",
    "provider_unavailable",
    "other",
}
MAX_OUTPUT = 4 * 1024 * 1024


class ContractError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise ContractError(message)


def load_json(path):
    require(Path(path).stat().st_size <= MAX_OUTPUT, "JSON document exceeds size limit")
    return json.loads(
        Path(path).read_text(),
        parse_constant=lambda x: (_ for _ in ()).throw(
            ContractError(f"invalid number {x}")
        ),
    )


def write_json(path, value):
    path = Path(path)
    tmp = path.with_name(path.name + ".tmp")
    tmp.write_text(
        json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n"
    )
    tmp.replace(path)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def command(value):
    require(
        isinstance(value, list) and 0 < len(value) <= 64,
        "command must be a nonempty argv array",
    )
    require(
        all(
            isinstance(v, str) and v and len(v) <= 4096 and "\0" not in v for v in value
        ),
        "invalid command argument",
    )
    return value


def validate_suite(suite):
    require(isinstance(suite, dict), "suite must be an object")
    require(suite.get("schema_version") == VERSION, "unsupported suite version")
    require(
        suite.get("repetitions") == 3 and suite.get("conditions") == CONDITIONS,
        "suite requires two conditions and three repetitions",
    )
    scenarios = suite.get("scenarios", [])
    require(len(scenarios) == 12, "suite requires twelve product scenarios")
    require(len({s.get("id") for s in scenarios}) == 12, "scenario ids must be unique")
    require(
        [s.get("language") for s in scenarios].count("python") == 6
        and [s.get("language") for s in scenarios].count("typescript") == 6,
        "suite must balance Python and TypeScript",
    )
    for scenario in scenarios:
        require(re.fullmatch(r"[a-z][a-z0-9-]+", scenario["id"]), "unsafe scenario id")
        require(
            scenario.get("requirements") and scenario.get("title"),
            "missing product requirements",
        )
        require(
            scenario.get("profile")
            == {"python": "python-api", "typescript": "typescript-web"}[
                scenario["language"]
            ],
            "profile differs from language",
        )
        checks = scenario.get("checks", [])
        require(
            len(checks) >= 6 and len({c.get("id") for c in checks}) == len(checks),
            "checks must be nonempty and uniquely identified",
        )
        require(
            sum(c.get("restart") is True for c in checks) == 1,
            "each product must test persistence across restart",
        )
        for check in checks:
            require(isinstance(check.get("id"), str), "check id required")
            if check.get("restart") is True:
                require(
                    set(check) == {"id", "restart"},
                    "restart check cannot contain a request",
                )
                continue
            require(
                check.get("method") in {"GET", "POST", "PUT", "PATCH", "DELETE"},
                "invalid HTTP method",
            )
            path = check.get("path", "")
            require(
                path.startswith("/")
                and not path.startswith("//")
                and not any(x in path for x in ("\r", "\n", "#", "://")),
                "request path must be local",
            )
            require(
                type(check.get("status")) is int and 200 <= check["status"] < 500,
                "invalid expected status",
            )
            assertions = check.get("assertions")
            if check.get("response_type", "json") == "html":
                require(
                    isinstance(assertions, dict)
                    and set(assertions)
                    == {
                        "language",
                        "title",
                        "heading",
                        "labelled_controls",
                        "submit_button",
                        "status_region",
                    }
                    and all(v is True for v in assertions.values()),
                    "HTML checks require the complete static accessibility surface contract",
                )
            else:
                require(
                    check.get("response_type", "json") == "json",
                    "unsupported response type",
                )
                require(
                    isinstance(assertions, dict)
                    and all(k == "" or k.startswith("/") for k in assertions),
                    "assertions must use JSON pointers",
                )
    return suite


def validate_config(config):
    require(isinstance(config, dict), "runner configuration must be an object")
    require(
        config.get("schema_version") == VERSION,
        "unsupported runner configuration version",
    )
    require(
        set(config)
        <= {
            "schema_version",
            "host",
            "host_version",
            "model_id",
            "command",
            "agentarch_bin",
            "runner_timeout_seconds",
        },
        "unknown runner configuration field",
    )
    require(
        config.get("host") in {"codex", "claude", "gemini"},
        "host must be codex, claude or gemini",
    )
    for key in ("host_version", "model_id"):
        value = config.get(key)
        require(
            isinstance(value, str)
            and value.strip()
            and value.lower() not in {"latest", "default", "auto"}
            and len(value) <= 200,
            f"explicit {key} required",
        )
    command(config.get("command"))
    timeout = config.get("runner_timeout_seconds", 1200)
    require(
        type(timeout) is int and 1 <= timeout <= 14400,
        "runner timeout must be 1..14400 seconds",
    )
    require(
        isinstance(config.get("agentarch_bin"), str) and config["agentarch_bin"],
        "agentarch_bin required to verify treated evidence",
    )
    return config


def matrix(suite, seed=20261116, selected=None):
    rows = []
    rng = random.Random(seed)
    for repetition in range(1, 4):
        scenarios = [
            s for s in suite["scenarios"] if not selected or s["id"] in selected
        ]
        rng.shuffle(scenarios)
        for scenario in scenarios:
            arms = list(CONDITIONS)
            rng.shuffle(arms)
            for condition in arms:
                rows.append(
                    {
                        "run_id": f"{scenario['id']}-{condition}-r{repetition}",
                        "scenario_id": scenario["id"],
                        "language": scenario["language"],
                        "condition": condition,
                        "repetition": repetition,
                    }
                )
    return rows


def stop_process(proc):
    # A process group is cleanup, not a sandbox; an adversarial process can escape it.
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=2)
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    proc.wait(timeout=2)


def run_command(argv, cwd, timeout, request=None):
    """Bound output/time and reap descendants; do not record arbitrary stderr or secrets."""
    command(argv)
    started = time.monotonic()
    with (
        tempfile.TemporaryFile() as out,
        tempfile.TemporaryFile() as err,
        tempfile.TemporaryFile() as inp,
    ):
        if request is not None:
            inp.write(json.dumps(request, allow_nan=False).encode())
            inp.seek(0)
        proc = subprocess.Popen(
            argv,
            cwd=cwd,
            stdin=inp,
            stdout=out,
            stderr=err,
            start_new_session=True,
            shell=False,
        )
        try:
            while proc.poll() is None:
                require(time.monotonic() - started < timeout, "command timed out")
                require(
                    os.fstat(out.fileno()).st_size <= MAX_OUTPUT
                    and os.fstat(err.fileno()).st_size <= MAX_OUTPUT,
                    "command output exceeds size limit",
                )
                time.sleep(0.02)
            require(
                os.fstat(out.fileno()).st_size <= MAX_OUTPUT
                and os.fstat(err.fileno()).st_size <= MAX_OUTPUT,
                "command output exceeds size limit",
            )
            require(
                proc.returncode == 0, f"command exited with status {proc.returncode}"
            )
            out.seek(0)
            return json.loads(
                out.read(MAX_OUTPUT),
                parse_constant=lambda x: (_ for _ in ()).throw(
                    ContractError(f"invalid number {x}")
                ),
            )
        finally:
            stop_process(proc)


def confined_path(root, relative):
    require(
        isinstance(relative, str) and relative and not Path(relative).is_absolute(),
        "artifact path must be relative",
    )
    root = Path(root).resolve()
    path = (root / relative).resolve()
    require(
        path.is_relative_to(root) and path != root, "artifact escapes run directory"
    )
    return path


def validate_response(response, config, run_dir):
    require(
        isinstance(response, dict) and response.get("schema_version") == VERSION,
        "invalid runner response",
    )
    allowed = {
        "schema_version",
        "status",
        "host",
        "host_version",
        "model_id",
        "candidate_dir",
        "start_command",
        "cost_usd",
        "agentarch_evidence",
        "host_error_category",
    }
    require(set(response) <= allowed, "unknown runner response field")
    for key in ("host", "host_version", "model_id"):
        require(
            response.get(key) == config[key],
            f"runner changed {key}; comparison would be confounded",
        )
    require(
        response.get("status") in {"completed", "failed", "inconclusive"},
        "runner must report completed, failed or inconclusive",
    )
    category = response.get("host_error_category")
    require(
        category is None
        or (isinstance(category, str) and category in HOST_ERROR_CATEGORIES),
        "invalid host error category",
    )
    require(
        category is None or response["status"] == "inconclusive",
        "host error category cannot be a product success or failure",
    )
    require("cost_usd" in response, "unknown cost must be explicitly null")
    cost = response["cost_usd"]
    require(
        cost is None
        or (type(cost) in {int, float} and math.isfinite(cost) and cost >= 0),
        "cost must be nonnegative or null",
    )
    if response["status"] == "completed":
        candidate = confined_path(run_dir, response.get("candidate_dir"))
        require(candidate.is_dir(), "candidate directory missing")
        command(response.get("start_command"))
        return candidate
    return None


def validate_evidence(export, status, profile, candidate):
    require(
        isinstance(status, dict) and status.get("initialized") is True,
        "CLI did not find an initialized workflow",
    )
    snap = status.get("snapshot") or {}
    state = snap.get("state") or {}
    fp = snap.get("fingerprint") or {}
    required = {"worktree", "git_dir", "head", "branch", "source_sha256", "profile"}
    require(
        set(fp) == required and all(isinstance(fp[k], str) and fp[k] for k in required),
        "current fingerprint missing",
    )
    require(
        Path(fp["worktree"]).resolve() == candidate.resolve()
        and fp["profile"] == profile,
        "workflow belongs to another project/profile",
    )
    require(
        re.fullmatch(r"[a-f0-9]{64}", fp["source_sha256"]), "invalid source fingerprint"
    )
    require(
        state.get("phase") == "pr_ready" and not snap.get("findings"),
        "workflow is not currently ready or has stale/failed findings",
    )
    require(
        export.get("schema_version") == VERSION
        and export.get("phase") == state["phase"],
        "evidence export phase/version differs from CLI",
    )
    product = snap.get("product") or {}
    require(
        product.get("executor_ids"),
        "treated workflow must declare development executor agents",
    )
    verification = state.get("verification") or {}
    review = state.get("review") or {}
    require(
        export.get("verification") == verification and export.get("review") == review,
        "export differs from CLI verification/review records",
    )
    require(
        verification.get("fingerprint") == fp and review.get("fingerprint") == fp,
        "verification or review fingerprint is stale",
    )
    require(
        verification.get("passed") is True
        and str(verification.get("isolation", "")).startswith("oci:"),
        "verification must pass with recorded OCI isolation",
    )
    checks = verification.get("checks")
    require(
        isinstance(checks, list)
        and checks
        and all(c.get("status") == "passed" for c in checks),
        "verification has missing or unsuccessful checks",
    )
    require(
        review.get("verdict") == "approved"
        and review.get("kind") in {"human", "independent_agent"},
        "independent approval required",
    )
    require(
        review.get("reviewer")
        and review["reviewer"] != product.get("author")
        and review.get("summary"),
        "review must name an independent reviewer and findings",
    )
    return fp


def verify_evidence(config, response, candidate, profile, artifact_dir, suffix):
    paths = response.get("agentarch_evidence")
    require(
        isinstance(paths, dict) and paths == {"export": EXPORT_PATH},
        "treated run must reference the CLI evidence export",
    )
    export_path = confined_path(candidate, paths["export"])
    export = load_json(export_path)
    status = run_command(
        [
            config["agentarch_bin"],
            "workflow",
            "status",
            "--root",
            str(candidate),
            "--format",
            "json",
        ],
        candidate,
        30,
    )
    fp = validate_evidence(export, status, profile, candidate)
    plugin = run_command(
        [
            config["agentarch_bin"],
            "plugin",
            "check",
            "--host",
            config["host"],
            "--root",
            str(candidate),
            "--format",
            "json",
        ],
        candidate,
        30,
    )
    require(
        plugin.get("host") == config["host"]
        and plugin.get("installed") is True
        and not plugin.get("issues"),
        "host plugin is missing or differs from its managed artifacts",
    )
    status_path = artifact_dir / f"workflow-status-{suffix}.json"
    write_json(status_path, status)
    export_copy = artifact_dir / f"workflow-evidence-{suffix}.json"
    write_json(export_copy, export)
    write_json(artifact_dir / f"plugin-check-{suffix}.json", plugin)
    return {
        "export": str(export_path),
        "export_sha256": sha256(export_path),
        "snapshot": str(status_path),
        "snapshot_sha256": sha256(status_path),
        "export_copy": str(export_copy),
        "fingerprint": fp,
    }


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class AccessibleSurface(HTMLParser):
    """Static surface check only; executed keyboard/interaction tests remain separate."""

    def __init__(self):
        super().__init__()
        self.language = False
        self.text = {"title": [], "h1": [], "button": []}
        self.active = []
        self.labels = set()
        self.controls = []
        self.submit = False
        self.status = False
        self.label_depth = 0

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag in self.text:
            self.active.append(tag)
        if tag == "html":
            self.language = bool(attrs.get("lang", "").strip())
        if tag == "label":
            self.label_depth += 1
            if attrs.get("for"):
                self.labels.add(attrs["for"])
        if tag in {"input", "select", "textarea"} and attrs.get("type", "text") not in {
            "hidden",
            "submit",
            "button",
        }:
            self.controls.append((attrs, self.label_depth > 0))
        if tag == "button" and attrs.get("type", "submit") == "submit":
            self.submit = True
        if (
            tag == "input"
            and attrs.get("type") == "submit"
            and attrs.get("value", "").strip()
        ):
            self.submit = True
        if attrs.get("role") == "status" or attrs.get("aria-live") in {
            "polite",
            "assertive",
        }:
            self.status = True

    def handle_endtag(self, tag):
        if tag in self.active:
            self.active.remove(tag)
        if tag == "label":
            self.label_depth = max(0, self.label_depth - 1)

    def handle_data(self, data):
        for tag in self.active:
            self.text[tag].append(data)

    def verify(self):
        checks = {
            "language": self.language,
            "title": bool("".join(self.text["title"]).strip()),
            "heading": bool("".join(self.text["h1"]).strip()),
            "labelled_controls": bool(self.controls)
            and all(
                wrapped
                or attrs.get("id") in self.labels
                or bool(attrs.get("aria-label", "").strip())
                for attrs, wrapped in self.controls
            ),
            "submit_button": self.submit,
            "status_region": self.status,
        }
        require(
            all(checks.values()),
            "HTML surface missing: "
            + ", ".join(k for k, ok in checks.items() if not ok),
        )


def http_request(port, check):
    body = json.dumps(check["body"]).encode() if "body" in check else None
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}{check['path']}",
        data=body,
        method=check["method"],
        headers={"Content-Type": "application/json"},
    )
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        response = opener.open(req, timeout=2)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        data = response.read(1024 * 1024 + 1)
        require(len(data) <= 1024 * 1024, "HTTP response exceeds size limit")
        require(
            response.status == check["status"],
            f"expected HTTP {check['status']}, received {response.status}",
        )
        if check.get("response_type") == "html":
            require(
                response.headers.get_content_type() == "text/html",
                "UI response is not text/html",
            )
            page = AccessibleSurface()
            page.feed(data.decode("utf-8"))
            page.verify()
            return
        require(
            response.headers.get_content_type() == "application/json",
            "response is not application/json",
        )
        parsed = json.loads(data)
    for pointer, expected in check.get("assertions", {}).items():
        actual = parsed
        for token in pointer.split("/")[1:] if pointer else []:
            token = token.replace("~1", "/").replace("~0", "~")
            actual = actual[int(token)] if isinstance(actual, list) else actual[token]
        require(
            type(actual) is type(expected) and actual == expected,
            f"JSON assertion failed at {pointer or '/'}",
        )


def acceptance(candidate, start_command, checks, run_dir, startup_timeout=15):
    require(os.name == "posix", "execution currently requires POSIX process groups")
    data_dir = run_dir / "data"
    data_dir.mkdir()
    home_dir = run_dir / "home"
    home_dir.mkdir()
    with socket.socket() as reserve:
        reserve.bind(("127.0.0.1", 0))
        port = reserve.getsockname()[1]
    env = {
        "PATH": os.environ.get("PATH", os.defpath),
        "HOME": str(home_dir),
        "LANG": "C.UTF-8",
        "PORT": str(port),
        "HOST": "127.0.0.1",
        "DATA_DIR": str(data_dir),
        "PYTHONDONTWRITEBYTECODE": "1",
        "PYTHONUNBUFFERED": "1",
    }
    results = []
    proc = None
    with tempfile.TemporaryFile() as logs:

        def launch():
            nonlocal proc
            proc = subprocess.Popen(
                start_command,
                cwd=candidate,
                stdin=subprocess.DEVNULL,
                stdout=logs,
                stderr=logs,
                env=env,
                shell=False,
                start_new_session=True,
            )
            deadline = time.monotonic() + startup_timeout
            while time.monotonic() < deadline:
                require(proc.poll() is None, "candidate exited during startup")
                require(
                    os.fstat(logs.fileno()).st_size <= MAX_OUTPUT,
                    "candidate output exceeds size limit",
                )
                try:
                    http_request(
                        port,
                        {
                            "method": "GET",
                            "path": "/health",
                            "status": 200,
                            "assertions": {"/ok": True},
                        },
                    )
                    return
                except (OSError, ValueError, KeyError, TypeError):
                    time.sleep(0.05)
            raise ContractError("candidate health check timed out")

        try:
            launch()
            for check in checks:
                started = time.monotonic()
                result = {"id": check["id"], "status": "passed"}
                try:
                    require(proc.poll() is None, "candidate exited")
                    require(
                        os.fstat(logs.fileno()).st_size <= MAX_OUTPUT,
                        "candidate output exceeds size limit",
                    )
                    if check.get("restart") is True:
                        stop_process(proc)
                        launch()
                    else:
                        http_request(port, check)
                except (OSError, ValueError, KeyError, TypeError, IndexError) as error:
                    result.update(status="failed", error=str(error)[:300])
                result["duration_ms"] = round((time.monotonic() - started) * 1000)
                results.append(result)
        except (OSError, ValueError) as error:
            results.append(
                {"id": "startup", "status": "failed", "error": str(error)[:300]}
            )
        finally:
            if proc:
                stop_process(proc)
    return results


def execute_run(row, scenario, suite, config, run_dir, seed):
    started = time.monotonic()
    run_dir.mkdir()
    request = {
        "schema_version": VERSION,
        "run": row,
        "host": config["host"],
        "host_version": config["host_version"],
        "model_id": config["model_id"],
        "seed": seed,
        "working_directory": str(run_dir),
        "runner_timeout_seconds": config.get("runner_timeout_seconds", 1200),
        "common_requirements": suite["common_requirements"],
        "scenario": scenario,
        "agentarch_bin": config["agentarch_bin"],
        "condition_requirements": "Use the installed host plugin and AgenteARQ workflow. Complete verify and independent review via CLI; return the CLI export path. Do not fabricate review."
        if row["condition"] == "agentarch"
        else "Use the same coding host/model without AgenteARQ skills, MCP, workflow or artifacts. Use an isolated host session/config to avoid treatment leakage.",
    }
    write_json(run_dir / "request.json", request)
    result = {
        **row,
        "status": "inconclusive",
        "cost_usd": None,
        "cost_source": "unknown",
        "checks": [],
        "agentarch_evidence": None,
    }
    try:
        runner_start = time.monotonic()
        response = run_command(
            config["command"],
            run_dir,
            config.get("runner_timeout_seconds", 1200),
            request,
        )
        result["runner_duration_ms"] = round((time.monotonic() - runner_start) * 1000)
        candidate = validate_response(response, config, run_dir)
        write_json(run_dir / "runner-response.json", response)
        result["cost_usd"] = response["cost_usd"]
        result["host_error_category"] = response.get("host_error_category")
        result["cost_source"] = (
            "reported_by_runner" if response["cost_usd"] is not None else "unknown"
        )
        if candidate is None:
            result.update(
                status=response["status"],
                error="external runner reported " + response["status"],
            )
        else:
            if row["condition"] == "agentarch":
                result["agentarch_evidence"] = {
                    "before": verify_evidence(
                        config,
                        response,
                        candidate,
                        scenario["profile"],
                        run_dir,
                        "before",
                    )
                }
            else:
                require(
                    not response.get("agentarch_evidence")
                    and not (candidate / "agentarch/project/product.yaml").exists(),
                    "executor-only condition contains AgenteARQ workflow evidence",
                )
            result["checks"] = acceptance(
                candidate, response["start_command"], scenario["checks"], run_dir
            )
            if row["condition"] == "agentarch":
                after = verify_evidence(
                    config, response, candidate, scenario["profile"], run_dir, "after"
                )
                require(
                    after["fingerprint"]
                    == result["agentarch_evidence"]["before"]["fingerprint"],
                    "candidate source changed during acceptance",
                )
                result["agentarch_evidence"]["after"] = after
            passed = len(result["checks"]) == len(scenario["checks"]) and all(
                c["status"] == "passed" for c in result["checks"]
            )
            result["status"] = "passed" if passed else "failed"
    except (
        OSError,
        ValueError,
        KeyError,
        TypeError,
        subprocess.SubprocessError,
    ) as error:
        result["error"] = str(error)[:500]
    result["duration_ms"] = round((time.monotonic() - started) * 1000)
    result["checks_passed"] = sum(c["status"] == "passed" for c in result["checks"])
    result["checks_expected"] = len(scenario["checks"])
    write_json(run_dir / "result.json", result)
    return result


def aggregate(results):
    summary = {}
    for condition in CONDITIONS:
        rows = [r for r in results if r["condition"] == condition]
        known = [r["cost_usd"] for r in rows if r["cost_usd"] is not None]
        summary[condition] = {
            "runs": len(rows),
            "passed": sum(r["status"] == "passed" for r in rows),
            "failed": sum(r["status"] == "failed" for r in rows),
            "inconclusive": sum(r["status"] == "inconclusive" for r in rows),
            "duration_ms": sum(r["duration_ms"] for r in rows),
            "reported_cost_usd": sum(known) if known else None,
            "runs_with_reported_cost": len(known),
            "runs_with_unknown_cost": len(rows) - len(known),
        }
    return summary


def update_progress(report, rows):
    completed = {row["run_id"] for row in report["results"]}
    reason = report.get("abort_reason") or "not_started"
    report["completed_runs"] = len(completed)
    report["not_run"] = [
        {**row, "status": "not_run", "reason": reason}
        for row in rows
        if row["run_id"] not in completed
    ]
    report["remaining_runs"] = len(report["not_run"])
    report["execution_complete"] = report["mode"] == "execute" and not report["not_run"]
    report["complete_suite"] = (
        report["matrix_complete"] and report["execution_complete"]
    )


def abort_reason(result):
    if result["status"] != "inconclusive":
        return None
    category = result.get("host_error_category")
    return "host_" + category if category else "infrastructure_inconclusive"


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", type=Path, default=SUITE_PATH)
    parser.add_argument("--runner-config", type=Path)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument(
        "--dry-run", action="store_true", help="validate and print the matrix; default"
    )
    mode.add_argument(
        "--execute",
        action="store_true",
        help="explicitly execute the configured external runner and candidate programs",
    )
    parser.add_argument(
        "--output-dir", type=Path, help="new directory, required for execution"
    )
    parser.add_argument("--seed", type=int, default=20261116)
    parser.add_argument(
        "--scenario",
        action="append",
        help="limit to scenario id; still runs both arms three times",
    )
    args = parser.parse_args(argv)
    try:
        suite = validate_suite(load_json(args.suite))
        if args.scenario:
            require(
                set(args.scenario) <= {s["id"] for s in suite["scenarios"]},
                "unknown scenario",
            )
        config = (
            validate_config(load_json(args.runner_config))
            if args.runner_config
            else None
        )
        rows = matrix(suite, args.seed, args.scenario)
        report = {
            "schema_version": VERSION,
            "mode": "execute" if args.execute else "dry_run",
            "suite": suite["suite"],
            "suite_sha256": sha256(args.suite),
            "harness_sha256": sha256(__file__),
            "seed": args.seed,
            "planned_runs": len(rows),
            "matrix_complete": len(rows) == 72,
            "complete_suite": False,
            "execution_complete": False,
            "aborted": False,
            "abort_reason": None,
            "abort_policy": "stop_on_inconclusive",
            "matrix": rows,
            "results": [],
            "summary": None,
            "limitations": [
                "Public API and static HTML contract tests measure a limited product slice; UI interaction/a11y execution is required in repository checks, not certified by the static HTML check.",
                "Host/model/version and costs are runner-reported; no causal quality claim follows from dry-run.",
                "The harness does not sandbox the runner or generated program. Execute in a dedicated sandbox.",
                "Workflow evidence proves current recorded checks/review, not the reviewer's actual independence or every host action.",
            ],
        }
        update_progress(report, rows)
        if args.execute:
            require(os.name == "posix", "execution requires POSIX")
            require(
                config is not None and args.output_dir is not None,
                "execution requires --runner-config and --output-dir",
            )
            binary = shutil.which(config["agentarch_bin"])
            require(binary is not None, "agentarch binary not found")
            config["agentarch_bin"] = str(Path(binary).resolve())
            report["runner"] = {k: v for k, v in config.items() if k != "command"}
            report["runner_command_sha256"] = hashlib.sha256(
                json.dumps(config["command"]).encode()
            ).hexdigest()
            report["agentarch_sha256"] = sha256(config["agentarch_bin"])
            output_dir = args.output_dir.resolve()
            require(
                not output_dir.exists(),
                "output directory already exists; preserve earlier results",
            )
            output_dir.mkdir(parents=True)
            write_json(output_dir / "report.json", report)
            by_id = {s["id"]: s for s in suite["scenarios"]}
            for row in rows:
                result = execute_run(
                    row,
                    by_id[row["scenario_id"]],
                    suite,
                    config,
                    output_dir / row["run_id"],
                    args.seed,
                )
                report["results"].append(result)
                report["summary"] = aggregate(report["results"])
                reason = abort_reason(result)
                if reason:
                    report["aborted"] = True
                    report["abort_reason"] = reason
                update_progress(report, rows)
                write_json(output_dir / "report.json", report)
                print(
                    f"{row['run_id']}: {result['status']}", file=sys.stderr, flush=True
                )
                if reason:
                    print(
                        f"benchmark stopped before the next run: {reason}",
                        file=sys.stderr,
                        flush=True,
                    )
                    break
        print(json.dumps(report, ensure_ascii=False, indent=2, allow_nan=False))
        return (
            0
            if not args.execute
            or all(r["status"] == "passed" for r in report["results"])
            else 1
        )
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"benchmark: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
