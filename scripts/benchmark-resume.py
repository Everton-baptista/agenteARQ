#!/usr/bin/env python3
"""Continue an immutable benchmark through its frozen harness; dry-run by default."""

from __future__ import annotations

import argparse
import copy
import fcntl
import hashlib
import importlib.util
import json
import os
import re
import sys
from pathlib import Path

REQUIRED = (
    "agentarch",
    "runner.json",
    "scripts/benchmark-products.py",
    "scripts/benchmark-host-runner.py",
    "benchmarks/products.json",
)
IMMUTABLE = (
    "schema_version", "mode", "suite", "suite_sha256", "harness_sha256",
    "seed", "planned_runs", "matrix_complete", "matrix", "runner",
    "runner_command_sha256", "agentarch_sha256", "abort_policy",
)
HOST_ERRORS = {
    "quota_exhausted", "rate_limited", "authentication_required",
    "network_unavailable", "provider_unavailable", "other",
}


class ResumeError(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise ResumeError(message)


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def json_hash(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, allow_nan=False).encode()).hexdigest()


def load_json(path):
    path = Path(path)
    require(path.is_file() and not path.is_symlink(), f"missing or redirected file: {path}")
    require(path.stat().st_size <= 8 * 1024 * 1024, "JSON document exceeds limit")
    return json.loads(path.read_text(), parse_constant=lambda value: (_ for _ in ()).throw(ResumeError(f"invalid number: {value}")))


def confined(root, path):
    path = Path(path)
    require(path.resolve().is_relative_to(root), "artifact escapes cohort")
    cursor = path
    while cursor != root:
        require(not cursor.is_symlink(), "redirected cohort artifact")
        cursor = cursor.parent
    return path


def active_processes(cohort):
    """Best-effort same-user Linux guard, in addition to the coordinator's flock."""
    require(Path("/proc").is_dir(), "resume requires Linux process inspection")
    prefix = str(cohort) + "/"
    found = []
    for proc in Path("/proc").glob("[0-9]*"):
        if int(proc.name) == os.getpid():
            continue
        try:
            args = (proc / "cmdline").read_bytes().decode(errors="replace").split("\0")
            if any(arg.startswith(prefix) for arg in args):
                found.append(int(proc.name))
        except (FileNotFoundError, ProcessLookupError, PermissionError):
            continue
    return found


def quota_evidence(run_dir, result):
    category = result.get("host_error_category")
    require(category is None or category in HOST_ERRORS, "unknown host error category")
    if result["status"] != "inconclusive":
        require(category is None, "terminal quality result has host error category")
        return False
    require(category == "quota_exhausted", "only native quota exhaustion is retryable; other infrastructure needs investigation")
    response = load_json(run_dir / "runner-response.json")
    execution = load_json(run_dir / "host-runner/execution.json")
    require(response.get("status") == execution.get("status") == "inconclusive", "quota status lacks runner evidence")
    require(response.get("host_error_category") == execution.get("host_error_category") == category, "quota category differs from runner evidence")
    require(any(s.get("host_error_category") == category and s.get("failed_events", 0) > 0 for s in execution.get("sessions", [])), "quota lacks a native failed host event")
    return True


def read_attempt(run_dir, rows, config, suite, seed):
    result_path = run_dir / "result.json"
    require(result_path.exists(), f"active or partial attempt requires reconciliation, never automatic retry: {run_dir}")
    result = load_json(result_path)
    run_id = result.get("run_id")
    require(run_id in rows, "result is not a row of the original matrix")
    require(all(result.get(k) == v for k, v in rows[run_id].items()), "result row changed")
    require(result.get("status") in {"passed", "failed", "inconclusive"}, "invalid result status")
    request = load_json(run_dir / "request.json")
    require(request.get("run") == rows[run_id] and request.get("seed") == seed, "request row or seed changed")
    require(request.get("working_directory") == str(run_dir), "request worktree binding changed")
    for key in ("host", "host_version", "model_id", "agentarch_bin"):
        require(request.get(key) == config[key], f"request changed {key}")
    require(request.get("runner_timeout_seconds") == config.get("runner_timeout_seconds", 1200), "request budget changed")
    scenario = next((s for s in suite["scenarios"] if s["id"] == rows[run_id]["scenario_id"]), None)
    require(request.get("scenario") == scenario and request.get("common_requirements") == suite["common_requirements"], "request contract changed")
    quota_evidence(run_dir, result)
    return {"run_id": run_id, "result": result, "path": str(result_path), "sha256": sha256(result_path)}


def effective_results(attempts, matrix):
    effective = {}
    for entry in attempts:
        previous = effective.get(entry["run_id"])
        if previous:
            require(previous["status"] == "inconclusive" and previous.get("host_error_category") == "quota_exhausted", "passed/failed or non-quota attempt cannot be retried")
        effective[entry["run_id"]] = entry["result"]
    return [effective[row["run_id"]] for row in matrix if row["run_id"] in effective]


def prepare(cohort, archive, retry_quota=False):
    """Read data and verify hashes only. Never import frozen Python on dry-run."""
    cohort, archive = Path(cohort).resolve(), Path(archive).resolve()
    require(not active_processes(cohort), "a process from this cohort is still active")
    freeze = load_json(cohort / "freeze.json")
    require(freeze == load_json(archive / "freeze.json"), "freeze differs from archived evidence")
    require(all(key in freeze for key in REQUIRED), "freeze lacks a required pre-recorded hash, including runner.json; no retroactive guarantee")
    for name, digest in freeze.items():
        path = confined(cohort, cohort / name)
        require(re.fullmatch(r"[a-f0-9]{64}", digest) and path.is_file() and sha256(path) == digest, f"frozen artifact changed: {name}")
    require(sha256(archive / "runner.json") == freeze["runner.json"], "saved runner configuration differs from frozen hash")
    config = load_json(cohort / "runner.json")
    suite = load_json(cohort / "benchmarks/products.json")
    base_path = cohort / "results/report.json"
    original = load_json(base_path)
    anchored = load_json(archive / "report.json")
    require(all(key in original and original[key] == anchored.get(key) for key in IMMUTABLE), "cohort parameters differ from archived report")
    require(original["mode"] == "execute" and original["abort_policy"] == "stop_on_inconclusive", "unsupported original report mode or abort policy")
    require(original["suite_sha256"] == freeze["benchmarks/products.json"] and original["harness_sha256"] == freeze["scripts/benchmark-products.py"] and original["agentarch_sha256"] == freeze["agentarch"], "report hashes disagree with freeze")
    require(original["runner"] == {k: v for k, v in config.items() if k != "command"}, "report runner parameters changed")
    require(original["runner_command_sha256"] == hashlib.sha256(json.dumps(config["command"]).encode()).hexdigest(), "runner argv changed")
    require(config["agentarch_bin"] == str(cohort / "agentarch") and str(cohort / "scripts/benchmark-host-runner.py") in config["command"], "runner does not use frozen cohort artifacts")
    matrix = original["matrix"]
    rows = {row["run_id"]: row for row in matrix}
    require(len(rows) == len(matrix) == original["planned_runs"], "duplicate or missing matrix rows")
    require(all(re.fullmatch(r"[a-z][a-z0-9_-]+", name) for name in rows), "unsafe run id")
    require(original["results"][:len(anchored["results"])] == anchored["results"], "previous archived outcomes changed")
    identity = {"freeze_sha256": sha256(cohort / "freeze.json"), "config_sha256": freeze["runner.json"], "harness_sha256": original["harness_sha256"], "runner_sha256": freeze["scripts/benchmark-host-runner.py"], "agentarch_sha256": original["agentarch_sha256"], "suite_sha256": original["suite_sha256"], "parameters_sha256": json_hash({k: original[k] for k in IMMUTABLE}), "base_report_sha256": sha256(base_path)}
    attempts = []
    for result in original["results"]:
        run_dir = confined(cohort, cohort / "results" / result["run_id"])
        entry = read_attempt(run_dir, rows, config, suite, original["seed"])
        require(entry["result"] == result, "report differs from saved result")
        require(entry["run_id"] not in {e["run_id"] for e in attempts}, "duplicate original result")
        attempts.append(entry)
    parent = {"path": str(base_path), "sha256": sha256(base_path)}
    recovered = []

    def recover(directory):
        known = {e["path"] for e in attempts}
        for run_dir in sorted(directory.iterdir()) if directory.exists() else []:
            if not run_dir.is_dir():
                continue
            confined(cohort, run_dir)
            require(run_dir.name in rows, "unknown attempt directory")
            if str(run_dir / "result.json") not in known:
                entry = read_attempt(run_dir, rows, config, suite, original["seed"])
                attempts.append(entry)
                recovered.append(entry["path"])

    recover(cohort / "results")
    continuation = cohort / "continuations"
    segments = sorted(continuation.glob("segment-*")) if continuation.exists() else []
    for segment in segments:
        confined(cohort, segment)
        require(re.fullmatch(r"segment-[0-9]{4}", segment.name), "invalid continuation segment")
        session = load_json(segment / "session.json")
        require(session["identity"] == identity and session["parent_snapshot"] == parent, "continuation identity or parent changed")
        snapshots = sorted(segment.glob("snapshot-*.json"))
        for index, snapshot in enumerate(snapshots):
            require(snapshot.name == f"snapshot-{index:04d}.json", "snapshot sequence has a gap")
            document = load_json(snapshot)
            metadata = document["_resume"]
            require(metadata["identity"] == identity and metadata["parent_snapshot"] == parent, "snapshot chain changed")
            recorded = metadata["attempts"]
            require(recorded[:len(attempts)] == attempts, "snapshot discarded or changed history")
            for entry in recorded:
                path = confined(cohort, Path(entry["path"]))
                actual = read_attempt(path.parent, rows, config, suite, original["seed"])
                require(actual == entry, "historical attempt changed")
            attempts = recorded
            require(document["results"] == effective_results(attempts, matrix), "snapshot effective outcomes changed")
            parent = {"path": str(snapshot), "sha256": sha256(snapshot)}
        recover(segment / "runs")
    effective = effective_results(attempts, matrix)
    by_id = {r["run_id"]: r for r in effective}
    quota = [r["run_id"] for r in effective if r["status"] == "inconclusive"]
    pending = [row for row in matrix if row["run_id"] not in by_id or (retry_quota and row["run_id"] in quota)]
    return {"cohort": str(cohort), "archive": str(archive), "retry_quota": retry_quota, "pending": pending, "preserved": effective, "attempts": attempts, "identity": identity, "parent_snapshot": parent, "recovered": recovered, "blocked": ["quota_retry_requires_flag"] if quota and not retry_quota else [], "original": original, "suite": suite, "config": config, "next_segment": len(segments) + 1}


def exclusive_json(path, value):
    # Open with O_EXCL: snapshots and history are never overwritten.
    data = (json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n").encode()
    with open(path, "xb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def load_harness(plan):
    path = Path(plan["cohort"]) / "scripts/benchmark-products.py"
    require(sha256(path) == plan["identity"]["harness_sha256"], "frozen harness changed before import")
    spec = importlib.util.spec_from_file_location("agentarch_frozen_benchmark", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.validate_suite(plan["suite"])
    module.validate_config(plan["config"])
    selected = {r["scenario_id"] for r in plan["original"]["matrix"]}
    require(module.matrix(plan["suite"], plan["original"]["seed"], selected) == plan["original"]["matrix"], "frozen matrix differs from saved order")
    return module


def execute(plan, harness=None):
    require(not plan["blocked"], "quota retry requires explicit --retry-quota")
    cohort = Path(plan["cohort"])
    parent_dir = cohort / "continuations"
    parent_dir.mkdir(exist_ok=True)
    with open(parent_dir / ".lock", "a") as lock:
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise ResumeError("another continuation owns the cohort lock") from error
        current = prepare(cohort, plan["archive"], plan["retry_quota"])
        require(current == plan, "cohort changed since the continuation plan")
        require(bool(plan["pending"]), "no eligible rows remain")
        module = harness if harness is not None else load_harness(plan)
        segment = parent_dir / f"segment-{plan['next_segment']:04d}"
        segment.mkdir()
        (segment / "runs").mkdir()
        exclusive_json(segment / "session.json", {"identity": plan["identity"], "parent_snapshot": plan["parent_snapshot"], "coordinator_sha256": sha256(__file__)})
        attempts = copy.deepcopy(plan["attempts"])
        previous = plan["parent_snapshot"]
        report = copy.deepcopy(plan["original"])
        report.update(aborted=False, abort_reason=None)
        sequence = 0

        def snapshot():
            nonlocal previous, sequence
            report["results"] = effective_results(attempts, report["matrix"])
            report["summary"] = module.aggregate(report["results"])
            report["completed_runs"] = len(report["results"])
            pending_ids = {r["run_id"] for r in report["results"]}
            report["not_run"] = [{**r, "status": "not_run", "reason": report.get("abort_reason") or "not_started"} for r in report["matrix"] if r["run_id"] not in pending_ids]
            report["remaining_runs"] = len(report["not_run"])
            report["execution_complete"] = not report["not_run"]
            report["complete_suite"] = report["matrix_complete"] and report["execution_complete"]
            report["_resume"] = {"identity": plan["identity"], "parent_snapshot": previous, "attempts": copy.deepcopy(attempts), "recovered": plan["recovered"], "coordinator_sha256": sha256(__file__), "attempts_summary": module.aggregate([e["result"] for e in attempts]), "limitation": "Effective outcomes count one row each. Attempts summary includes earlier quota attempts, their time and known costs. Unknown costs remain unknown. A fixed host-catalog model ID does not ensure the provider stayed unchanged during a pause."}
            path = segment / f"snapshot-{sequence:04d}.json"
            exclusive_json(path, report)
            previous = {"path": str(path), "sha256": sha256(path)}
            sequence += 1

        snapshot()
        scenarios = {s["id"]: s for s in plan["suite"]["scenarios"]}
        for row in plan["pending"]:
            # The frozen execute_run still owns all model calls, gates and acceptance.
            result = module.execute_run(row, scenarios[row["scenario_id"]], plan["suite"], plan["config"], segment / "runs" / row["run_id"], report["seed"])
            entry = read_attempt(segment / "runs" / row["run_id"], {r["run_id"]: r for r in report["matrix"]}, plan["config"], plan["suite"], report["seed"])
            require(entry["result"] == result, "frozen harness result differs from persisted artifact")
            attempts.append(entry)
            reason = module.abort_reason(result)
            if reason:
                report.update(aborted=True, abort_reason=reason)
            snapshot()
            if reason:
                break
        return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cohort", type=Path, required=True)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--retry-quota", action="store_true")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--execute", action="store_true")
    mode.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    try:
        plan = prepare(args.cohort, args.archive, args.retry_quota)
        if args.execute:
            report = execute(plan)
            print(json.dumps(report, ensure_ascii=False, indent=2, allow_nan=False))
            return 0 if report["complete_suite"] and all(r["status"] == "passed" for r in report["results"]) else 1
        print(json.dumps({k: plan[k] for k in ("cohort", "identity", "parent_snapshot", "pending", "blocked", "recovered")}, ensure_ascii=False, indent=2))
        return 0
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"benchmark-resume: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
