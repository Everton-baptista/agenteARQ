"""Quota/abort regressions using synthetic events and local fixtures; no AI calls."""

import argparse
import contextlib
import importlib.util
import io
import json
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

SCRIPTS = Path(__file__).resolve().parents[1]


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


runner = load("quota_host_runner", "benchmark-host-runner.py")
bench = load("quota_products", "benchmark-products.py")


def empty_stats():
    return {
        "thread_id": None,
        "usage": {},
        "mcp_tools": [],
        "item_types": {},
        "failed_events": 0,
    }


class HostErrorTests(unittest.TestCase):
    def test_native_error_codes_distinguish_quota_rate_auth_network_and_provider(self):
        cases = {
            "insufficient_quota": "quota_exhausted",
            "usage_limit_reached": "quota_exhausted",
            "rate_limit_exceeded": "rate_limited",
            "invalid_api_key": "authentication_required",
            "connection_error": "network_unavailable",
            "overloaded_error": "provider_unavailable",
        }
        for kind in ("error", "turn.failed"):
            for code, category in cases.items():
                with self.subTest(kind=kind, code=code):
                    value = runner.classify_host_error(
                        {"type": kind, "error": {"code": code, "message": "fixture"}}
                    )
                    self.assertEqual(category, value["category"])
                    self.assertEqual(code, value["code"])
                    self.assertRegex(value["sha256"], r"^[a-f0-9]{64}$")

    def test_native_message_fallback_and_priority_do_not_confuse_quota_with_rate(self):
        cases = [
            ({"message": "You have hit your usage limit"}, "quota_exhausted"),
            ({"message": "Too many requests"}, "rate_limited"),
            ({"message": "Please run codex login"}, "authentication_required"),
            (
                {"code": "rate_limit_exceeded", "message": "Insufficient quota"},
                "quota_exhausted",
            ),
            (
                {"code": "invalid_api_key", "message": "Rate limit exceeded"},
                "authentication_required",
            ),
            (
                {"code": "insufficient_quota", "message": "Authentication required"},
                "quota_exhausted",
            ),
        ]
        for payload, expected in cases:
            with self.subTest(payload=payload):
                self.assertEqual(
                    expected,
                    runner.classify_host_error(
                        {"type": "turn.failed", "error": payload}
                    )["category"],
                )

    def test_model_tool_and_success_events_never_become_host_quota(self):
        error = {"code": "insufficient_quota", "message": "quota exhausted"}
        events = [
            {"type": "assistant", "error": error},
            {"type": "turn.completed", "error": error},
            {
                "type": "item.completed",
                "item": {
                    "type": "agent_message",
                    "text": "quota exhausted",
                    "error": error,
                },
            },
            {
                "type": "item.completed",
                "item": {
                    "type": "mcp_tool_call",
                    "server": "agentarch",
                    "status": "completed",
                    "result": {"isError": True, "error": error},
                },
            },
            {
                "type": "item.completed",
                "item": {
                    "type": "command_execution",
                    "aggregated_output": "quota exhausted",
                    "exit_code": 1,
                },
            },
        ]
        for event in events:
            with self.subTest(event=event):
                stats = empty_stats()
                self.assertIsNone(runner.classify_host_error(event))
                runner.consume_event(event, stats)
                self.assertEqual(0, stats["failed_events"])
                self.assertIsNone(stats.get("host_error_category"))

    def test_unknown_codes_and_malformed_payloads_are_other_without_raw_retention(self):
        canary = "synthetic-private-error-canary-98765"
        for payload in [
            None,
            73,
            [],
            {"code": ["insufficient_quota"], "message": {}},
            {"code": canary, "message": canary},
        ]:
            with self.subTest(payload=payload):
                value = runner.classify_host_error({"type": "error", "error": payload})
                self.assertEqual("other", value["category"])
                self.assertIsNone(value["code"])
                self.assertEqual({"category", "code", "sha256"}, set(value))
                self.assertNotIn(canary, json.dumps(value))
        for event in [None, [], "not an event"]:
            with self.subTest(event=event), self.assertRaises(runner.RunnerError):
                runner.consume_event(event, empty_stats())

    def test_event_priority_preserves_quota_after_less_specific_failures(self):
        stats = empty_stats()
        for code in [
            "rate_limit_exceeded",
            "insufficient_quota",
            "connection_error",
            "unrecognized_code",
        ]:
            runner.consume_event({"type": "error", "error": {"code": code}}, stats)
        self.assertEqual(4, stats["failed_events"])
        self.assertEqual("quota_exhausted", stats["host_error_category"])
        self.assertEqual("insufficient_quota", stats["host_error_code"])

    def test_failed_local_host_keeps_usage_and_execution_evidence_without_error_content(
        self,
    ):
        with tempfile.TemporaryDirectory(
            prefix="agentarch-quota-fixture-"
        ) as temporary:
            root = Path(temporary)
            candidate = root / "host-candidate"
            artifacts = root / "host-artifacts"
            candidate.mkdir()
            artifacts.mkdir()
            canary = "synthetic-private-error-canary-98765"
            events = [
                {"type": "thread.started", "thread_id": "fixture-session"},
                {
                    "type": "turn.completed",
                    "usage": {
                        "input_tokens": 100,
                        "cached_input_tokens": 40,
                        "output_tokens": 8,
                    },
                },
                {
                    "type": "error",
                    "error": {"code": "insufficient_quota", "message": canary},
                },
            ]
            host = root / "local-host-fixture"
            host.write_text(
                "#!"
                + sys.executable
                + "\nimport json,sys\nsys.stdin.read()\nfor event in "
                + repr(events)
                + ": print(json.dumps(event),flush=True)\nsys.exit(7)\n"
            )
            host.chmod(0o700)
            request = {
                "schema_version": "1.0",
                "run": {"condition": "executor_only"},
                "host": "codex",
                "host_version": "fixture-1",
                "model_id": "fixture-model-pinned",
                "working_directory": str(root),
                "runner_timeout_seconds": 1200,
                "agentarch_bin": "/fixture/agentarch",
                "common_requirements": "A synthetic local JSON API",
                "scenario": {
                    "profile": "python-api",
                    "language": "python",
                    "title": "Fixture",
                    "requirements": "Synthetic task",
                    "checks": [],
                },
            }
            with self.assertRaises(runner.HostRunError) as caught:
                runner.run_host(
                    str(host),
                    request,
                    candidate,
                    artifacts,
                    "implement-1",
                    "synthetic prompt",
                    time.monotonic() + 5,
                    5,
                )
            failure = caught.exception
            self.assertIn("exit 7", str(failure))
            self.assertNotIn(canary, str(failure))
            self.assertEqual("fixture-session", failure.stats["thread_id"])
            self.assertEqual(events[1]["usage"], failure.stats["usage"])
            self.assertEqual("quota_exhausted", failure.stats["host_error_category"])
            self.assertGreaterEqual(failure.stats["duration_ms"], 0)
            self.assertRegex(failure.stats["event_stream_sha256"], r"^[a-f0-9]{64}$")
            metadata = json.loads((artifacts / "implement-1-metadata.json").read_text())
            self.assertEqual(failure.stats, metadata)
            self.assertNotIn(canary, json.dumps(metadata))
            self.assertFalse(list(artifacts.glob("*.jsonl")))
            self.assertFalse((artifacts / "implement-1-answer.json").exists())

            args = argparse.Namespace(
                host_binary=str(host),
                disable_plugin=[],
                verifier_image_python="fixture@sha256:" + "a" * 64,
                verifier_image_typescript=None,
                max_attempts=3,
                implementation_seconds=60,
            )

            def trusted(argv, *unused, **kwargs):
                if "check-ignore" in argv:
                    return runner.TEMP_DIRECTORY + "/agentarch-ignore-probe\n"
                return "fixture-1" if "--version" in argv else ""

            with (
                contextlib.chdir(root),
                mock.patch.object(runner, "trusted_command", side_effect=trusted),
                mock.patch.object(runner, "global_skill_inventory", return_value=[]),
                mock.patch.object(runner, "plugin_inventory", return_value=[]),
                mock.patch.object(
                    runner.shutil, "which", return_value="/fixture/docker"
                ),
                mock.patch.object(runner, "run_host", side_effect=failure) as invoke,
            ):
                response = runner.run(request, args)
            self.assertEqual(1, invoke.call_count)
            self.assertEqual("inconclusive", response["status"])
            self.assertEqual("quota_exhausted", response["host_error_category"])
            execution = json.loads((root / "host-runner/execution.json").read_text())
            self.assertEqual([metadata], execution["sessions"])
            self.assertEqual(68, execution["usage"]["budget_tokens"])
            self.assertFalse(execution["usage_incomplete"])
            self.assertNotIn(canary, json.dumps(execution))


class CohortAbortTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="agentarch-abort-fixture-")
        self.root = Path(self.temporary.name)
        self.suite = bench.validate_suite(bench.load_json(bench.SUITE_PATH))
        self.config = {
            "schema_version": "1.0",
            "host": "codex",
            "host_version": "fixture-1",
            "model_id": "fixture-model-pinned",
            "command": [sys.executable, "must-not-run.py"],
            "agentarch_bin": sys.executable,
            "runner_timeout_seconds": 10,
        }
        self.config_path = self.root / "config.json"
        bench.write_json(self.config_path, self.config)

    def tearDown(self):
        self.temporary.cleanup()

    def run_main(
        self,
        *,
        full=False,
        dry=False,
        status="inconclusive",
        category="quota_exhausted",
    ):
        args = [
            "--dry-run" if dry else "--execute",
            "--runner-config",
            str(self.config_path),
            "--output-dir",
            str(self.root / "output"),
        ]
        if not full:
            args += ["--scenario", self.suite["scenarios"][0]["id"]]

        def fake_run(row, *unused):
            value = {**row, "status": status, "cost_usd": None, "duration_ms": 1}
            if category is not None:
                value["host_error_category"] = category
            return value

        stdout, stderr = io.StringIO(), io.StringIO()
        with (
            mock.patch.object(bench, "execute_run", side_effect=fake_run) as execute,
            mock.patch.object(
                bench,
                "run_command",
                side_effect=AssertionError(
                    "external model/process invocation forbidden"
                ),
            ),
            contextlib.redirect_stdout(stdout),
            contextlib.redirect_stderr(stderr),
        ):
            code = bench.main(args)
        self.assertTrue(stdout.getvalue(), stderr.getvalue())
        return code, json.loads(stdout.getvalue()), execute.call_count

    def test_first_quota_aborts_six_runs_without_spending_on_remaining_five(self):
        code, report, calls = self.run_main()
        self.assertEqual(
            (1, 1, 6, 1, 5),
            (
                code,
                calls,
                report["planned_runs"],
                report["completed_runs"],
                report["remaining_runs"],
            ),
        )
        self.assertTrue(report["aborted"])
        self.assertEqual("host_quota_exhausted", report["abort_reason"])
        self.assertFalse(report["matrix_complete"])
        self.assertFalse(report["execution_complete"])
        self.assertFalse(report["complete_suite"])
        self.assertEqual(5, len(report["not_run"]))
        self.assertTrue(
            all(
                row["status"] == "not_run" and row["reason"] == "host_quota_exhausted"
                for row in report["not_run"]
            )
        )
        self.assertEqual(
            report, json.loads((self.root / "output/report.json").read_text())
        )
        self.assertEqual(
            {row["run_id"] for row in report["matrix"]},
            {row["run_id"] for row in report["results"] + report["not_run"]},
        )

    def test_first_quota_in_full_matrix_leaves_seventy_one_unrun(self):
        code, report, calls = self.run_main(full=True)
        self.assertEqual(
            (1, 1, 72, 1, 71),
            (
                code,
                calls,
                report["planned_runs"],
                report["completed_runs"],
                report["remaining_runs"],
            ),
        )
        self.assertTrue(report["matrix_complete"])
        self.assertFalse(report["execution_complete"])
        self.assertFalse(report["complete_suite"])

    def test_dry_run_makes_zero_calls_and_never_claims_execution_complete(self):
        code, report, calls = self.run_main(full=True, dry=True)
        self.assertEqual(
            (0, 0, 0, 72),
            (code, calls, report["completed_runs"], report["remaining_runs"]),
        )
        self.assertTrue(report["matrix_complete"])
        self.assertFalse(report["aborted"])
        self.assertFalse(report["execution_complete"])
        self.assertFalse(report["complete_suite"])
        self.assertFalse((self.root / "output").exists())

    def test_quality_failure_does_not_abort_remaining_product_attempts(self):
        code, report, calls = self.run_main(status="failed", category=None)
        self.assertEqual(
            (1, 6, 6, 0),
            (code, calls, report["completed_runs"], report["remaining_runs"]),
        )
        self.assertFalse(report["aborted"])
        self.assertIsNone(report["abort_reason"])
        self.assertTrue(report["execution_complete"])
        self.assertFalse(report["complete_suite"])
        self.assertEqual([], report["not_run"])
        self.assertEqual(6, sum(row["failed"] for row in report["summary"].values()))

    def test_unknown_infrastructure_inconclusive_also_stops(self):
        code, report, calls = self.run_main(category=None)
        self.assertEqual((1, 1, 5), (code, calls, report["remaining_runs"]))
        self.assertEqual("infrastructure_inconclusive", report["abort_reason"])

    def test_response_rejects_unknown_category_and_category_on_product_success_or_failure(
        self,
    ):
        response = {
            "schema_version": "1.0",
            "host": self.config["host"],
            "host_version": self.config["host_version"],
            "model_id": self.config["model_id"],
            "status": "inconclusive",
            "cost_usd": None,
        }
        for category in ["unrecognized", "secret-canary", 1, True]:
            with (
                self.subTest(category=category),
                self.assertRaises(bench.ContractError),
            ):
                bench.validate_response(
                    {**response, "host_error_category": category},
                    self.config,
                    self.root,
                )
        for status in ["completed", "failed"]:
            with self.subTest(status=status), self.assertRaises(bench.ContractError):
                bench.validate_response(
                    {
                        **response,
                        "status": status,
                        "host_error_category": "quota_exhausted",
                    },
                    self.config,
                    self.root,
                )
        for category in bench.HOST_ERROR_CATEGORIES:
            with self.subTest(category=category):
                self.assertIsNone(
                    bench.validate_response(
                        {**response, "host_error_category": category},
                        self.config,
                        self.root,
                    )
                )


if __name__ == "__main__":
    unittest.main()
