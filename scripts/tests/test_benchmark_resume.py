"""Resume-coordinator contracts using synthetic files and an injected runner, never models."""

import contextlib
import copy
import hashlib
import importlib.util
import io
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


SCRIPTS = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("benchmark_resume_tests", SCRIPTS / "benchmark-resume.py")
resume = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(resume)


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, allow_nan=False) + "\n")


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class FixtureHarness:
    """Imitates only the three harness methods used by execute()."""

    def __init__(self, fixture, statuses=(), crash_after_result=False):
        self.fixture = fixture
        self.statuses = iter(statuses)
        self.crash_after_result = crash_after_result
        self.calls = []

    def execute_run(self, row, scenario, suite, config, run_dir, seed):
        self.calls.append(row["run_id"])
        status, category, cost = next(self.statuses, ("passed", None, None))
        value = self.fixture.write_attempt(row, status, category, cost, Path(run_dir))
        if self.crash_after_result:
            raise RuntimeError("fixture crash after durable result, before snapshot")
        return value

    @staticmethod
    def aggregate(results):
        return {
            condition: {
                "runs": sum(r["condition"] == condition for r in results),
                "passed": sum(r["condition"] == condition and r["status"] == "passed" for r in results),
                "failed": sum(r["condition"] == condition and r["status"] == "failed" for r in results),
                "inconclusive": sum(r["condition"] == condition and r["status"] == "inconclusive" for r in results),
            }
            for condition in ("executor_only", "agentarch")
        }

    @staticmethod
    def abort_reason(result):
        if result["status"] != "inconclusive":
            return None
        category = result.get("host_error_category")
        return "host_" + category if category else "infrastructure_inconclusive"


class ResumeTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="agentarch-resume-unit-")
        self.root = Path(self.temporary.name)
        self.cohort = self.root / "cohort"
        self.archive = self.root / "archive"
        self.cohort.mkdir()
        self.archive.mkdir()
        # This is data only. The frozen Python files deliberately cannot be imported.
        self.suite = json.loads((SCRIPTS.parent / "benchmarks/products.json").read_text())
        self.scenario = self.suite["scenarios"][0]
        self.rows = [
            {"run_id": f"{self.scenario['id']}-{condition}-r{repetition}",
             "scenario_id": self.scenario["id"], "language": self.scenario["language"],
             "condition": condition, "repetition": repetition}
            for repetition in (1, 2, 3)
            for condition in ("agentarch", "executor_only")
        ]
        self.config = {
            "schema_version": "1.0", "host": "codex", "host_version": "fixture-1",
            "model_id": "fixture-model-pinned", "runner_timeout_seconds": 1200,
            "command": [sys.executable, str(self.cohort / "scripts/benchmark-host-runner.py")],
            "agentarch_bin": str(self.cohort / "agentarch"),
        }
        for name in ("scripts/benchmark-products.py", "scripts/benchmark-host-runner.py"):
            path = self.cohort / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("raise RuntimeError('FROZEN_IMPORT_FORBIDDEN')\n")
        (self.cohort / "agentarch").write_bytes(b"synthetic binary; never execute\n")
        (self.cohort / "agentarch").chmod(0o700)
        write_json(self.cohort / "benchmarks/products.json", self.suite)
        write_json(self.cohort / "runner.json", self.config)
        write_json(self.archive / "runner.json", self.config)
        self.freeze = {name: digest(self.cohort / name) for name in (
            "agentarch", "runner.json", "scripts/benchmark-products.py",
            "scripts/benchmark-host-runner.py", "benchmarks/products.json")}
        write_json(self.archive / "freeze.json", self.freeze)
        self.report = {
            "schema_version": "1.0", "mode": "execute", "suite": self.suite["suite"],
            "suite_sha256": self.freeze["benchmarks/products.json"],
            "harness_sha256": self.freeze["scripts/benchmark-products.py"],
            "agentarch_sha256": self.freeze["agentarch"], "seed": 41,
            "runner": {k: v for k, v in self.config.items() if k != "command"},
            "runner_command_sha256": hashlib.sha256(json.dumps(self.config["command"]).encode()).hexdigest(),
            "matrix": self.rows, "planned_runs": 6, "matrix_complete": False,
            "complete_suite": False, "execution_complete": False,
            "aborted": False, "abort_reason": None, "abort_policy": "stop_on_inconclusive",
            "results": [], "summary": None, "limitations": ["Synthetic unit fixture"],
        }
        self.report["results"] = [
            self.write_attempt(self.rows[0], "passed", cost=0.25),
            self.write_attempt(self.rows[1], "failed"),
        ]
        self.save_reports()
        self.process_patch = mock.patch.object(resume, "active_processes", return_value=[])
        self.process_patch.start()
        self.addCleanup(self.process_patch.stop)
        self.addCleanup(self.temporary.cleanup)

    def save_reports(self):
        complete = {r["run_id"] for r in self.report["results"]}
        self.report["completed_runs"] = len(complete)
        self.report["not_run"] = [{**row, "status": "not_run", "reason": "not_started"}
                                  for row in self.rows if row["run_id"] not in complete]
        self.report["remaining_runs"] = len(self.report["not_run"])
        self.report["summary"] = FixtureHarness.aggregate(self.report["results"])
        write_json(self.cohort / "results/report.json", self.report)
        write_json(self.archive / "report.json", self.report)

    def write_attempt(self, row, status, category=None, cost=None, directory=None):
        directory = directory or self.cohort / "results" / row["run_id"]
        directory.mkdir(parents=True, exist_ok=False)
        checks = [] if status == "inconclusive" else [
            {"id": check["id"], "status": "failed" if status == "failed" and i == 0 else "passed"}
            for i, check in enumerate(self.scenario["checks"])]
        result = {**row, "status": status, "host_error_category": category,
                  "cost_usd": cost, "cost_source": "unknown" if cost is None else "reported_by_runner",
                  "duration_ms": 11, "runner_duration_ms": 7, "checks": checks,
                  "checks_passed": sum(c["status"] == "passed" for c in checks),
                  "checks_expected": len(self.scenario["checks"]), "agentarch_evidence": None}
        request = {"schema_version": "1.0", "run": row, "host": self.config["host"],
                   "host_version": self.config["host_version"], "model_id": self.config["model_id"],
                   "seed": 41, "working_directory": str(directory), "runner_timeout_seconds": 1200,
                   "common_requirements": self.suite["common_requirements"], "scenario": self.scenario,
                   "agentarch_bin": self.config["agentarch_bin"], "condition_requirements": "Synthetic unit fixture"}
        response = {"schema_version": "1.0", "host": self.config["host"],
                    "host_version": self.config["host_version"], "model_id": self.config["model_id"],
                    "status": "inconclusive" if category else "completed" if status == "passed" else "failed",
                    "host_error_category": category, "cost_usd": cost}
        write_json(directory / "request.json", request)
        write_json(directory / "result.json", result)
        write_json(directory / "runner-response.json", response)
        if category:
            write_json(directory / "host-runner/execution.json", {
                "schema_version": "1.0", "host": self.config["host"], "model_id": self.config["model_id"],
                "status": "inconclusive", "host_error_category": category,
                "sessions": [{"failed_events": 1, "host_error_category": category,
                              "host_error_code": "insufficient_quota" if category == "quota_exhausted" else "connection_error",
                              "usage": {"input_tokens": 100, "output_tokens": 4}}]})
        return result

    def add_quota(self, category="quota_exhausted", cost=0.5):
        self.report["results"].append(self.write_attempt(self.rows[2], "inconclusive", category, cost))
        self.report.update(aborted=True, abort_reason="host_" + category)
        self.save_reports()

    def prepare(self, retry_quota=False):
        return resume.prepare(self.cohort, self.archive, retry_quota=retry_quota)

    def old_hashes(self):
        return {str(p.relative_to(self.root)): digest(p)
                for base in (self.archive, self.cohort / "results") for p in base.rglob("*") if p.is_file()}

    def test_default_dry_run_never_imports_frozen_code_or_writes_files(self):
        before = self.old_hashes()
        output, errors = io.StringIO(), io.StringIO()
        with mock.patch.object(resume, "execute", side_effect=AssertionError("execution forbidden")), \
             contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            code = resume.main(["--cohort", str(self.cohort), "--archive", str(self.archive)])
        self.assertEqual(0, code, errors.getvalue())
        self.assertTrue(output.getvalue())
        self.assertEqual(before, self.old_hashes())
        self.assertFalse((self.cohort / "continuations").exists())

    def test_prepare_preserves_terminal_results_and_original_matrix_order(self):
        before = self.old_hashes()
        plan = self.prepare()
        self.assertEqual(self.report["results"], plan["preserved"])
        self.assertEqual(self.rows[2:], plan["pending"])
        self.assertEqual(before, self.old_hashes())
        self.assertFalse((self.cohort / "continuations").exists())

    def test_existing_quota_requires_opt_in_before_execute(self):
        self.add_quota()
        plan = self.prepare()
        self.assertIn("quota_retry_requires_flag", plan["blocked"])
        self.assertEqual(self.rows[3:], plan["pending"])
        harness = FixtureHarness(self)
        with self.assertRaises(resume.ResumeError):
            resume.execute(plan, harness=harness)
        self.assertEqual([], harness.calls)
        self.assertEqual(self.rows[2:], self.prepare(retry_quota=True)["pending"])

    def test_retry_requires_matching_native_quota_evidence(self):
        self.add_quota()
        directory = self.cohort / "results" / self.rows[2]["run_id"]
        evidence = directory / "host-runner/execution.json"
        original = json.loads(evidence.read_text())
        for mutation in ("missing", "category", "session_category", "failed_events"):
            with self.subTest(mutation=mutation):
                value = copy.deepcopy(original)
                if mutation == "missing":
                    evidence.unlink()
                elif mutation == "category":
                    value["host_error_category"] = "network_unavailable"
                    write_json(evidence, value)
                elif mutation == "session_category":
                    value["sessions"][0]["host_error_category"] = "other"
                    write_json(evidence, value)
                else:
                    value["sessions"][0]["failed_events"] = 0
                    write_json(evidence, value)
                with self.assertRaises(resume.ResumeError):
                    self.prepare(retry_quota=True)
                write_json(evidence, original)
        response_path = directory / "runner-response.json"
        response = json.loads(response_path.read_text())
        response["host_error_category"] = "rate_limited"
        write_json(response_path, response)
        with self.assertRaises(resume.ResumeError):
            self.prepare(retry_quota=True)

    def test_other_infrastructure_failure_cannot_be_retried_as_quota(self):
        self.add_quota(category="network_unavailable")
        with self.assertRaises(resume.ResumeError):
            self.prepare(retry_quota=True)

    def test_frozen_file_drift_is_rejected(self):
        for name in self.freeze:
            with self.subTest(file=name):
                path = self.cohort / name
                original = path.read_bytes()
                path.write_bytes(original + b"\n")
                with self.assertRaises(resume.ResumeError):
                    self.prepare()
                path.write_bytes(original)

    def test_report_matrix_and_runner_configuration_drift_are_rejected(self):
        path = self.cohort / "results/report.json"
        for field in ("matrix", "seed", "runner", "runner_command_sha256"):
            with self.subTest(field=field):
                value = copy.deepcopy(self.report)
                if field == "matrix":
                    value[field] = list(reversed(value[field]))
                elif field == "seed":
                    value[field] += 1
                elif field == "runner":
                    value[field]["model_id"] = "different-model"
                else:
                    value[field] = "0" * 64
                write_json(path, value)
                with self.assertRaises(resume.ResumeError):
                    self.prepare()
                write_json(path, self.report)

    def test_active_cohort_or_partial_attempt_blocks_resume(self):
        with mock.patch.object(resume, "active_processes", return_value=[os.getpid()]):
            with self.assertRaises(resume.ResumeError):
                self.prepare()
        directory = self.cohort / "results" / self.rows[2]["run_id"]
        write_json(directory / "request.json", {"run": self.rows[2]})
        with self.assertRaises(resume.ResumeError):
            self.prepare()

    def test_result_must_match_its_request_row(self):
        path = self.cohort / "results" / self.rows[0]["run_id"] / "request.json"
        value = json.loads(path.read_text())
        value["run"] = self.rows[1]
        write_json(path, value)
        with self.assertRaises(resume.ResumeError):
            self.prepare()

    def test_execute_preserves_old_results_and_stops_on_new_quota(self):
        before = self.old_hashes()
        harness = FixtureHarness(self, [("inconclusive", "quota_exhausted", 0.75)])
        report = resume.execute(self.prepare(), harness=harness)
        self.assertEqual([self.rows[2]["run_id"]], harness.calls)
        self.assertTrue(report["aborted"])
        self.assertEqual("host_quota_exhausted", report["abort_reason"])
        self.assertFalse(report["execution_complete"])
        self.assertEqual(before, self.old_hashes())
        self.assertEqual(self.report["results"], report["results"][:2])

    def test_orphan_result_recovers_without_duplicate_model_attempt(self):
        original = self.old_hashes()
        first = FixtureHarness(self, crash_after_result=True)
        with self.assertRaisesRegex(RuntimeError, "fixture crash"):
            resume.execute(self.prepare(), harness=first)
        self.assertEqual([self.rows[2]["run_id"]], first.calls)
        plan = self.prepare()
        self.assertTrue(plan["recovered"])
        self.assertEqual(self.rows[3:], plan["pending"])
        second = FixtureHarness(self)
        report = resume.execute(plan, harness=second)
        self.assertEqual([r["run_id"] for r in self.rows[3:]], second.calls)
        self.assertEqual([r["run_id"] for r in self.rows], [r["run_id"] for r in report["results"]])
        self.assertEqual(6, len({r["run_id"] for r in report["results"]}))
        self.assertEqual(original, self.old_hashes())
        self.assertTrue(report["execution_complete"])

    def test_quota_retry_history_keeps_old_costs_and_unknown_costs(self):
        self.add_quota(cost=0.5)
        original = self.old_hashes()
        harness = FixtureHarness(self, [("passed", None, 1.25)])
        report = resume.execute(self.prepare(retry_quota=True), harness=harness)
        attempts = report["_resume"]["attempts"]
        quota_attempts = [a for a in attempts if a["run_id"] == self.rows[2]["run_id"]]
        self.assertEqual(["inconclusive", "passed"], [a["result"]["status"] for a in quota_attempts])
        self.assertEqual([0.5, 1.25], [a["result"]["cost_usd"] for a in quota_attempts])
        self.assertTrue(any(a["result"]["cost_usd"] is None for a in attempts))
        self.assertEqual(original, self.old_hashes())
        self.assertEqual([r["run_id"] for r in self.rows[2:]], harness.calls)
        self.assertEqual(6, len(report["results"]))


if __name__ == "__main__":
    unittest.main()
