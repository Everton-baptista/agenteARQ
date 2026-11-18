"""Harness regressions with controlled local processes only; no AI/provider calls."""

import copy
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "scripts/benchmark-products.py"
spec = importlib.util.spec_from_file_location("benchmark_products", SCRIPT)
bench = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bench)

SERVER = r"""
import json, os
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
root=Path(os.environ['DATA_DIR']); root.mkdir(exist_ok=True)
state=root/'counter.json'
class Handler(BaseHTTPRequestHandler):
 def log_message(self, *args): pass
 def do_GET(self):
  if self.path=='/health': result={'ok':True}
  elif self.path=='/wrong': result={'value':False}
  else:
   count=json.loads(state.read_text()) if state.exists() else 0
   result={'value':count}
  self.send_response(200); self.send_header('Content-Type','application/json'); self.end_headers()
  self.wfile.write(json.dumps(result).encode())
 def do_POST(self):
  count=json.loads(state.read_text()) if state.exists() else 0
  state.write_text(json.dumps(count+1))
  self.send_response(201); self.send_header('Content-Type','application/json'); self.end_headers()
  self.wfile.write(json.dumps({'value':count+1}).encode())
HTTPServer.allow_reuse_address=True
HTTPServer((os.environ['HOST'],int(os.environ['PORT'])), Handler).serve_forever()
"""


class BenchmarkTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.suite = bench.validate_suite(bench.load_json(bench.SUITE_PATH))
        self.config = {
            "schema_version": "1.0",
            "host": "codex",
            "host_version": "fixture-1.0",
            "model_id": "test-model-2026-01-01",
            "command": [sys.executable, "fixture.py"],
            "agentarch_bin": sys.executable,
            "runner_timeout_seconds": 10,
        }

    def tearDown(self):
        self.tmp.cleanup()

    def response(self):
        return {
            "schema_version": "1.0",
            "host": "codex",
            "host_version": "fixture-1.0",
            "model_id": "test-model-2026-01-01",
            "status": "completed",
            "candidate_dir": "candidate",
            "start_command": [sys.executable, "server.py"],
            "cost_usd": None,
        }

    def test_matrix_balanced_reproducible_and_not_workflow_scenarios(self):
        rows = bench.matrix(self.suite)
        self.assertEqual(72, len(rows))
        self.assertEqual(rows, bench.matrix(self.suite))
        self.assertNotEqual(rows, bench.matrix(self.suite, 123))
        self.assertEqual(12, len({r["scenario_id"] for r in rows}))
        self.assertEqual(72, len({r["run_id"] for r in rows}))
        for scenario in self.suite["scenarios"]:
            for condition in bench.CONDITIONS:
                self.assertEqual(
                    3,
                    sum(
                        r["scenario_id"] == scenario["id"]
                        and r["condition"] == condition
                        for r in rows
                    ),
                )

    def test_dry_run_does_not_execute_external_runner(self):
        marker = self.root / "executed"
        self.config["command"] = [
            sys.executable,
            "-c",
            f'open({str(marker)!r},"w").write("bad")',
        ]
        config = self.root / "config.json"
        bench.write_json(config, self.config)
        value = bench.run_command(
            [sys.executable, str(SCRIPT), "--dry-run", "--runner-config", str(config)],
            self.root,
            10,
        )
        self.assertEqual(72, value["planned_runs"])
        self.assertEqual([], value["results"])
        self.assertIsNone(value["summary"])
        self.assertFalse(marker.exists())

    def test_invalid_suite_refuses_remote_requests_and_duplicates(self):
        bad = copy.deepcopy(self.suite)
        bad["scenarios"][0]["checks"][0]["path"] = "//evil.example/"
        with self.assertRaises(bench.ContractError):
            bench.validate_suite(bad)
        bad = copy.deepcopy(self.suite)
        bad["scenarios"][1]["id"] = bad["scenarios"][0]["id"]
        with self.assertRaises(bench.ContractError):
            bench.validate_suite(bad)

    def test_config_requires_argv_and_explicit_model(self):
        self.assertEqual(self.config, bench.validate_config(self.config))
        for key, value in [
            ("command", "bash script.sh"),
            ("model_id", "latest"),
            ("host", "invented"),
            ("runner_timeout_seconds", True),
        ]:
            bad = {**self.config, key: value}
            with self.assertRaises(bench.ContractError):
                bench.validate_config(bad)

    def test_runner_path_escape_model_mismatch_and_cost_contract(self):
        (self.root / "candidate").mkdir()
        response = self.response()
        self.assertEqual(
            self.root / "candidate",
            bench.validate_response(response, self.config, self.root),
        )
        for key, value in [
            ("candidate_dir", "../outside"),
            ("candidate_dir", str(self.root)),
            ("model_id", "different-model"),
            ("cost_usd", -1),
            ("cost_usd", True),
            ("cost_usd", float("nan")),
        ]:
            with self.assertRaises(bench.ContractError):
                bench.validate_response(
                    {**response, key: value}, self.config, self.root
                )
        del response["cost_usd"]
        with self.assertRaises(bench.ContractError):
            bench.validate_response(response, self.config, self.root)

    def test_timeout_and_output_limit_do_not_become_success(self):
        with self.assertRaisesRegex(bench.ContractError, "timed out"):
            bench.run_command(
                [sys.executable, "-c", "import time; time.sleep(30)"], self.root, 0.05
            )
        with self.assertRaisesRegex(bench.ContractError, "size limit"):
            bench.run_command(
                [sys.executable, "-c", f'print("x"*{bench.MAX_OUTPUT + 1})'],
                self.root,
                10,
            )

    def test_argv_has_no_shell_interpolation(self):
        literal = "$(touch injected); `touch injected2`"
        value = bench.run_command(
            [
                sys.executable,
                "-c",
                "import json,sys;print(json.dumps(sys.argv[1]))",
                literal,
            ],
            self.root,
            10,
        )
        self.assertEqual(literal, value)
        self.assertFalse((self.root / "injected").exists())
        self.assertFalse((self.root / "injected2").exists())

    def evidence(self):
        fp = {
            "worktree": str(self.root),
            "git_dir": str(self.root / ".git"),
            "head": "abc123",
            "branch": "main",
            "source_sha256": "a" * 64,
            "profile": "python-api",
        }
        verification = {
            "fingerprint": fp,
            "passed": True,
            "isolation": "oci:image@sha256:" + "b" * 64,
            "checks": [{"id": "tests", "status": "passed"}],
        }
        review = {
            "fingerprint": fp,
            "verdict": "approved",
            "kind": "independent_agent",
            "reviewer": "reviewer",
            "summary": "Acceptance inspected",
        }
        export = {
            "schema_version": "1.0",
            "phase": "pr_ready",
            "verification": verification,
            "review": review,
        }
        status = {
            "initialized": True,
            "snapshot": {
                "state": {
                    "phase": "pr_ready",
                    "verification": verification,
                    "review": review,
                },
                "fingerprint": fp,
                "findings": [],
                "product": {"author": "implementer", "executor_ids": ["dev-codex"]},
            },
        }
        return export, status

    def test_current_evidence_requires_export_verify_and_independent_review(self):
        export, status = self.evidence()
        self.assertEqual(
            status["snapshot"]["fingerprint"],
            bench.validate_evidence(export, status, "python-api", self.root),
        )
        cases = []
        bad = copy.deepcopy(status)
        bad["snapshot"]["findings"] = [{"id": "evidence.stale"}]
        cases.append((export, bad))
        bad = copy.deepcopy(export)
        bad["verification"]["fingerprint"]["source_sha256"] = "c" * 64
        cases.append((bad, status))
        bad = copy.deepcopy(status)
        bad["snapshot"]["product"]["executor_ids"] = []
        cases.append((export, bad))
        bad = copy.deepcopy(status)
        bad["snapshot"]["product"]["author"] = "reviewer"
        cases.append((export, bad))
        bad_export = copy.deepcopy(export)
        bad_status = copy.deepcopy(status)
        bad_export["verification"]["passed"] = False
        bad_status["snapshot"]["state"]["verification"]["passed"] = False
        cases.append((bad_export, bad_status))
        for exp, st in cases:
            with self.assertRaises(bench.ContractError):
                bench.validate_evidence(exp, st, "python-api", self.root)

    def test_real_http_assertions_restart_and_durability(self):
        candidate = self.root / "candidate"
        candidate.mkdir()
        (candidate / "server.py").write_text(SERVER)
        checks = [
            {
                "id": "increment",
                "method": "POST",
                "path": "/value",
                "status": 201,
                "assertions": {"/value": 1},
            },
            {"id": "restart", "restart": True},
            {
                "id": "persisted",
                "method": "GET",
                "path": "/value",
                "status": 200,
                "assertions": {"/value": 1},
            },
            {
                "id": "false-is-not-zero",
                "method": "GET",
                "path": "/wrong",
                "status": 200,
                "assertions": {"/value": 0},
            },
        ]
        results = bench.acceptance(
            candidate, [sys.executable, "server.py"], checks, self.root
        )
        self.assertEqual(
            ["passed", "passed", "passed", "failed"], [r["status"] for r in results]
        )

    def test_html_surface_is_checked_and_not_confused_with_browser_execution(self):
        page = bench.AccessibleSurface()
        page.feed(
            '<html lang="en"><title>Tasks</title><h1>Tasks</h1><form><label for="title">Task title</label><input id="title"><button type="submit">Create</button></form><p role="status"></p></html>'
        )
        page.verify()
        missing = bench.AccessibleSurface()
        missing.feed("<html><title>Tasks</title><input><button>Create</button></html>")
        with self.assertRaises(bench.ContractError):
            missing.verify()
        self.assertEqual("products-web-v2", self.suite["suite"])
        self.assertTrue(
            all(
                any(c.get("response_type") == "html" for c in s["checks"])
                for s in self.suite["scenarios"]
            )
        )

    def test_missing_startup_is_a_failure(self):
        results = bench.acceptance(
            self.root,
            [sys.executable, "-c", "raise SystemExit(1)"],
            [],
            self.root,
            startup_timeout=1,
        )
        self.assertEqual("failed", results[0]["status"])
        self.assertEqual("startup", results[0]["id"])

    def test_treated_run_cannot_pass_without_evidence(self):
        runner = self.root / "runner.py"
        response = self.response()
        runner.write_text(
            'import pathlib,json,sys\nr=json.load(sys.stdin)\npathlib.Path("candidate").mkdir()\nprint('
            + repr(json.dumps(response))
            + ")\n"
        )
        self.config["command"] = [sys.executable, str(runner)]
        row = {
            "run_id": "trial",
            "scenario_id": "python-tasks",
            "language": "python",
            "condition": "agentarch",
            "repetition": 1,
        }
        result = bench.execute_run(
            row,
            self.suite["scenarios"][0],
            self.suite,
            self.config,
            self.root / "trial",
            1,
        )
        self.assertEqual("inconclusive", result["status"])
        self.assertIn("export", result["error"])
        self.assertEqual([], result["checks"])
        self.assertIsNone(result["cost_usd"])

    def test_external_runner_and_http_result_end_to_end_without_ai(self):
        runner = self.root / "fixture_runner.py"
        response = self.response()
        runner.write_text(
            "import pathlib,json,sys\n"
            "request=json.load(sys.stdin)\n"
            'pathlib.Path("candidate").mkdir()\n'
            'pathlib.Path("candidate/server.py").write_text(' + repr(SERVER) + ")\n"
            "print(" + repr(json.dumps(response)) + ")\n"
        )
        self.config["command"] = [sys.executable, str(runner)]
        row = {
            "run_id": "local-fixture",
            "scenario_id": "controlled-counter",
            "language": "python",
            "condition": "executor_only",
            "repetition": 1,
        }
        scenario = {
            "profile": "python-api",
            "checks": [
                {
                    "id": "increment",
                    "method": "POST",
                    "path": "/value",
                    "status": 201,
                    "assertions": {"/value": 1},
                },
                {"id": "restart", "restart": True},
                {
                    "id": "persisted",
                    "method": "GET",
                    "path": "/value",
                    "status": 200,
                    "assertions": {"/value": 1},
                },
            ],
        }
        result = bench.execute_run(
            row, scenario, self.suite, self.config, self.root / "trial", 1
        )
        self.assertEqual("passed", result["status"], result)
        self.assertEqual(3, result["checks_passed"])
        self.assertIsNone(result["cost_usd"])
        self.assertEqual(result, bench.load_json(self.root / "trial/result.json"))

    def test_runner_infrastructure_failure_preserves_inconclusive(self):
        response = self.response()
        response["status"] = "inconclusive"
        self.assertIsNone(bench.validate_response(response, self.config, self.root))
        fixture = self.root / "infrastructure_fixture.py"
        fixture.write_text(
            "import sys,json\njson.load(sys.stdin)\nprint("
            + repr(json.dumps(response))
            + ")\n"
        )
        self.config["command"] = [sys.executable, str(fixture)]
        row = {
            "run_id": "infra",
            "scenario_id": "python-tasks",
            "condition": "executor_only",
            "language": "python",
            "repetition": 1,
        }
        result = bench.execute_run(
            row,
            self.suite["scenarios"][0],
            self.suite,
            self.config,
            self.root / "infra",
            1,
        )
        self.assertEqual("inconclusive", result["status"])
        self.assertEqual([], result["checks"])

    def test_cost_summary_keeps_unknown_cost_distinct(self):
        rows = [
            {
                "condition": "executor_only",
                "status": "passed",
                "duration_ms": 100,
                "cost_usd": None,
            },
            {
                "condition": "executor_only",
                "status": "failed",
                "duration_ms": 10,
                "cost_usd": 2.5,
            },
            {
                "condition": "agentarch",
                "status": "inconclusive",
                "duration_ms": 1,
                "cost_usd": None,
            },
        ]
        summary = bench.aggregate(rows)
        self.assertEqual(2.5, summary["executor_only"]["reported_cost_usd"])
        self.assertEqual(1, summary["executor_only"]["runs_with_unknown_cost"])
        self.assertIsNone(summary["agentarch"]["reported_cost_usd"])
        self.assertEqual(1, summary["agentarch"]["inconclusive"])


if __name__ == "__main__":
    unittest.main()
