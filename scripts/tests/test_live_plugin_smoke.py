"""Parser and boundary tests use deterministic events/processes, never a model."""
import importlib.util
import contextlib
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location("live_plugin_smoke", Path(__file__).resolve().parents[1] / "live-plugin-smoke.py")
smoke = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(smoke)


def process(exit_code=0):
    return {"exit_code": exit_code, "timed_out": False, "output_limit": False}


class TraceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="agentarch-smoke-unit-")
        self.project = Path(self.temp.name) / "project"
        self.project.mkdir()

    def tearDown(self):
        self.temp.cleanup()

    def events(self, host):
        target = str(smoke.skill_path(self.project, host))
        if host == "codex":
            return [
                {"type": "item.completed", "item": {"type": "command_execution", "command": "/bin/bash -lc 'cat " + target + "'", "exit_code": 0, "status": "completed"}},
                *[{"type": "item.completed", "item": {"type": "mcp_tool_call", "server": "agentarch", "tool": tool, "status": "completed", "error": None, "result": {"isError": False}}} for tool in smoke.READ_TOOLS],
                {"type": "turn.completed", "usage": {"input_tokens": 100, "output_tokens": 20, "cached_input_tokens": 50}},
            ]
        if host == "claude":
            events = [{"type": "system", "subtype": "init", "mcp_servers": [{"name": "agentarch", "status": "connected"}], "skills": [smoke.SKILL]}]
            calls = [("Read", {"file_path": target}), *[("mcp__agentarch__" + name, {}) for name in smoke.READ_TOOLS]]
            for index, (name, args) in enumerate(calls):
                events += [{"type": "assistant", "message": {"model": "claude-sonnet-5-5", "content": [{"type": "tool_use", "id": str(index), "name": name, "input": args}]}},
                           {"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": str(index), "is_error": False, "content": "PRIVATE_TOOL_CONTENT"}]}}]
            return events + [{"type": "result", "subtype": "success", "is_error": False, "result": "PRIVATE_COMPLETION", "usage": {"input_tokens": 100, "output_tokens": 20}, "total_cost_usd": .012}]
        events = [{"type": "init", "model": "gemini-3.5-flash"}]
        calls = [("activate_skill", {"name": smoke.SKILL}), *[("agentarch_" + name, {}) for name in smoke.READ_TOOLS]]
        for index, (name, params) in enumerate(calls):
            events += [{"type": "tool_use", "tool_id": str(index), "tool_name": name, "parameters": params},
                       {"type": "tool_result", "tool_id": str(index), "status": "success", "output": "PRIVATE_TOOL_CONTENT"}]
        return events + [{"type": "result", "status": "success", "stats": {"input_tokens": 100, "output_tokens": 20, "models": {"gemini-3.5-flash": {"output_tokens": 20}}}}]

    def parse(self, host, events):
        return smoke.parse_trace(host, "\n".join(json.dumps(event) for event in events), self.project)

    def test_native_skill_and_two_correlated_reads_pass_for_each_host(self):
        for host in smoke.PROVIDERS:
            with self.subTest(host=host):
                trace = self.parse(host, self.events(host))
                self.assertTrue(smoke.trace_passed(trace, process()))
                self.assertEqual(trace["skill_load"], "passed")
                self.assertEqual(set(trace["successful_mcp_reads"]), set(smoke.READ_TOOLS))
                self.assertNotIn("PRIVATE", json.dumps(trace))
                self.assertNotIn(str(self.project), json.dumps(trace))
                self.assertNotIn("command", trace)
        self.assertEqual(self.parse("claude", self.events("claude"))["cost_usd"], .012)
        self.assertIsNone(self.parse("codex", self.events("codex"))["cost_usd"])

    def test_discovery_and_claimed_final_text_are_not_tool_execution(self):
        events = [{"type": "system", "subtype": "init", "mcp_servers": [{"name": "agentarch", "status": "connected"}], "skills": [smoke.SKILL]},
                  {"type": "result", "subtype": "success", "is_error": False, "result": "Read skill and both MCP tools successfully", "usage": {"output_tokens": 30}}]
        trace = self.parse("claude", events)
        self.assertEqual(trace["mcp_discovery"], "connected")
        self.assertEqual(trace["skill_discovery"], "discovered")
        self.assertFalse(smoke.trace_passed(trace, process()))

    def test_codex_completed_with_embedded_error_never_passes(self):
        for field, value in [("result", {"isError": True}), ("result", {"is_error": True}), ("error", {}), ("result", {"isError": "false"})]:
            events = self.events("codex")
            events[1]["item"][field] = value
            trace = self.parse("codex", events)
            self.assertFalse(smoke.trace_passed(trace, process()))
            self.assertEqual(trace["failed_tool_calls"], 1)

    def test_claude_success_subtype_with_is_error_true_is_failure(self):
        events = self.events("claude")
        events[-1]["is_error"] = True
        self.assertFalse(smoke.trace_passed(self.parse("claude", events), process()))
        events[-1]["is_error"] = False
        events[4]["message"]["content"][0]["is_error"] = True
        self.assertFalse(smoke.trace_passed(self.parse("claude", events), process()))

    def test_correlated_results_required_and_duplicate_ids_rejected(self):
        for host in ("claude", "gemini"):
            events = self.events(host)
            del events[2]
            self.assertFalse(smoke.trace_passed(self.parse(host, events), process()))
            events = self.events(host)
            events.insert(2, events[1])
            self.assertFalse(smoke.trace_passed(self.parse(host, events), process()))

    def test_gemini_success_with_error_payload_is_failure(self):
        events = self.events("gemini")
        events[4]["error"] = {"message": "PRIVATE_ERROR"}
        trace = self.parse("gemini", events)
        self.assertFalse(smoke.trace_passed(trace, process()))
        self.assertNotIn("PRIVATE", json.dumps(trace))

    def test_malformed_event_shapes_do_not_crash_or_pass(self):
        for host in smoke.PROVIDERS:
            trace = self.parse(host, [{"type": []}, {"type": "system", "subtype": "init", "mcp_servers": None, "skills": None},
                                      {"type": "tool_result", "tool_id": {}, "status": "success"},
                                      {"type": "item.completed", "item": {"type": []}}])
            self.assertFalse(smoke.trace_passed(trace, process()))

    def test_exit_timeout_source_change_and_missing_usage_fail_closed(self):
        trace = self.parse("codex", self.events("codex"))
        self.assertFalse(smoke.trace_passed(trace, process(1)))
        self.assertFalse(smoke.trace_passed(trace, {**process(), "timed_out": True}))
        self.assertFalse(smoke.trace_passed(trace, process(), unchanged=False))
        trace["usage"] = {}
        self.assertFalse(smoke.trace_passed(trace, process()))

    def test_mutating_or_unrelated_tool_attempt_prevents_pass(self):
        events = self.events("codex")
        events.insert(0, {"type": "item.completed", "item": {"type": "mcp_tool_call", "server": "agentarch", "tool": "record_checkpoint", "status": "completed"}})
        trace = self.parse("codex", events)
        self.assertEqual(trace["unexpected_tool_calls"], 1)
        self.assertFalse(smoke.trace_passed(trace, process()))

    def test_codex_auxiliary_types_are_counted_without_content(self):
        events = self.events("codex")
        for kind in smoke.CODEX_AUXILIARY_ITEMS:
            events.insert(0, {"type": "item.completed", "item": {"type": kind, "text": "PRIVATE_CONTENT"}})
        trace = self.parse("codex", events)
        summary = trace["codex_event_classification"]
        self.assertEqual(summary["auxiliary_items"], 3)
        self.assertEqual(summary["completed_item_counts"]["mcp_tool_call"], 2)
        self.assertTrue(smoke.trace_passed(trace, process()))
        self.assertNotIn("PRIVATE", json.dumps(summary))

    def test_codex_unknown_types_remain_blockers_and_only_export_hashes(self):
        events = self.events("codex") + [
            {"type": "item.completed", "item": {"type": "PRIVATE_UNKNOWN_TYPE", "text": "PRIVATE_CONTENT"}},
            {"type": "PRIVATE_UNKNOWN_EVENT", "secret": "PRIVATE_VALUE"},
        ]
        trace = self.parse("codex", events)
        summary = trace["codex_event_classification"]
        self.assertEqual(summary["unclassified_items"], 1)
        self.assertEqual(summary["unclassified_events"], 1)
        self.assertEqual(len(summary["unknown_type_hash_counts"]), 2)
        self.assertNotIn("PRIVATE", json.dumps(summary))
        self.assertFalse(smoke.trace_passed(trace, process()))

    def test_codex_prohibited_started_tool_without_completion_fails(self):
        for kind in ("command_execution", "file_change", "web_search", "dynamic_tool_call", "tool_search"):
            events = self.events("codex")
            events.insert(0, {"type": "item.started", "item": {"id": "PRIVATE_ID", "type": kind, "command": "PRIVATE_COMMAND"}})
            trace = self.parse("codex", events)
            summary = trace["codex_event_classification"]
            self.assertEqual(summary["prohibited_tool_attempts"], 1)
            self.assertEqual(summary["incomplete_tool_items"], 1)
            self.assertEqual(summary["started_item_counts"][kind], 1)
            self.assertNotIn("PRIVATE", json.dumps(summary))
            self.assertFalse(smoke.trace_passed(trace, process()))

    def test_codex_lifecycle_counts_one_attempt_and_rejects_duplicate_completion(self):
        item = {"id": "PRIVATE_ID", "type": "web_search"}
        events = [{"type": kind, "item": item} for kind in ("item.started", "item.updated", "item.completed")]
        summary = smoke.classify_codex_events("\n".join(map(json.dumps, events)))
        self.assertEqual(summary["prohibited_tool_attempts"], 1)
        self.assertEqual(summary["incomplete_tool_items"], 0)
        self.assertEqual(summary["invalid_item_lifecycle"], 0)
        events.append(events[-1])
        summary = smoke.classify_codex_events("\n".join(map(json.dumps, events)))
        self.assertEqual(summary["invalid_item_lifecycle"], 1)

    def test_codex_checkpoint_allowlist_is_explicit_and_does_not_allow_shell(self):
        item = {"id": "PRIVATE_ID", "type": "mcp_tool_call", "server": "agentarch", "tool": "record_checkpoint"}
        raw = json.dumps({"type": "item.completed", "item": item})
        self.assertFalse(smoke.codex_classification_passed(smoke.classify_codex_events(raw)))
        self.assertTrue(smoke.codex_classification_passed(smoke.classify_codex_events(raw, ("get_workflow", "record_checkpoint"))))
        raw += "\n" + json.dumps({"type": "item.completed", "item": {"type": "command_execution", "command": "cat " + str(smoke.skill_path(self.project, "codex"))}})
        summary = smoke.classify_codex_events(raw, ("get_workflow", "record_checkpoint"))
        self.assertEqual(summary["prohibited_tool_attempts"], 1)

    def test_codex_incomplete_allowed_tool_and_changed_identity_fail(self):
        item = {"id": "PRIVATE_ID", "type": "mcp_tool_call", "server": "agentarch", "tool": "get_workflow"}
        events = [{"type": "item.started", "item": item}]
        summary = smoke.classify_codex_events(json.dumps(events[0]))
        self.assertEqual(summary["incomplete_tool_items"], 1)
        self.assertFalse(smoke.codex_classification_passed(summary))
        events.append({"type": "item.completed", "item": {**item, "tool": "get_next_task"}})
        summary = smoke.classify_codex_events("\n".join(map(json.dumps, events)))
        self.assertEqual(summary["invalid_item_lifecycle"], 1)

    def test_codex_error_item_and_bounded_unknown_metadata_fail(self):
        for kind in ("error", "turn.failed"):
            summary = smoke.classify_codex_events(json.dumps({"type": kind, "message": "PRIVATE_ERROR"}))
            self.assertEqual(summary["host_error_events"], 1)
            self.assertFalse(smoke.codex_classification_passed(summary))
        events = self.events("codex") + [{"type": "item.completed", "item": {"type": "error", "message": "PRIVATE_ERROR"}}]
        trace = self.parse("codex", events)
        self.assertEqual(trace["codex_event_classification"]["host_error_items"], 1)
        self.assertFalse(smoke.trace_passed(trace, process()))
        events = [{"type": "item.completed", "item": {"type": "PRIVATE_" + str(i)}} for i in range(40)]
        summary = smoke.classify_codex_events("\n".join(map(json.dumps, events)))
        self.assertEqual(summary["unclassified_items"], 40)
        self.assertEqual(len(summary["unknown_type_hash_counts"]), 32)
        self.assertTrue(summary["metadata_limit_exceeded"])
        self.assertNotIn("PRIVATE", json.dumps(summary))

    def test_skill_path_mention_is_not_a_read(self):
        target = smoke.skill_path(self.project, "codex")
        for command in ["echo " + str(target), "cat " + str(target) + "; echo fake", "/tmp/cat " + str(target), "cat /other/.agents/skills/agentarch-discover-product/SKILL.md"]:
            self.assertFalse(smoke.command_reads_skill(command, target, self.project))
        self.assertTrue(smoke.command_reads_skill("cat -- " + str(target), target, self.project))

    def test_only_numeric_allowlisted_metadata_survives(self):
        events = self.events("claude")
        events[-1]["usage"].update({"output_tokens": True, "api_key": "SECRET", "arbitrary": 42})
        events[-1]["total_cost_usd"] = float("nan")
        trace = self.parse("claude", events)
        self.assertEqual(trace["usage"], {"input_tokens": 100})
        self.assertIsNone(trace["cost_usd"])
        self.assertNotIn("SECRET", json.dumps(trace))

    def test_auth_metadata_drops_account_fields_and_credentials(self):
        with mock.patch.object(smoke, "run", return_value=(process(), json.dumps({"loggedIn": True, "email": "private@example.invalid", "token": "secret-value"}), "")):
            auth = smoke.auth_status("claude", "claude", self.project)
        self.assertEqual(auth["status"], "authenticated")
        self.assertNotIn("private", json.dumps(auth))
        self.assertNotIn("token", json.dumps(auth))

    def test_commands_keep_host_safety_and_explicit_managed_transport(self):
        saved = {key: os.environ.get(key) for key in ("HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR")}
        server = {"command": "/tmp/agentarch", "args": ["mcp", "serve", "--root", str(self.project)]}
        for host in smoke.PROVIDERS:
            args = smoke.host_command(host, "/bin/host", "model-version", self.project, server)
            for bad in ("--ignore-rules", "--skip-trust", "--yolo", "--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox"):
                self.assertNotIn(bad, args)
            if host == "codex":
                self.assertIn("read-only", args)
                self.assertIn('mcp_servers.agentarch.command="/tmp/agentarch"', args)
                allow = next(arg for arg in args if arg.startswith("mcp_servers.agentarch.enabled_tools="))
                self.assertNotIn("record_checkpoint", allow)
            if host == "claude":
                self.assertIn("Read,Skill", args)
                self.assertIn("dontAsk", args)
            if host == "gemini":
                self.assertIn("plan", args)
        self.assertEqual(saved, {key: os.environ.get(key) for key in saved})

    def test_missing_auth_never_invokes_model_or_reports_pass(self):
        output = Path(self.temp.name) / "output"
        binary = Path(self.temp.name) / "binary"
        binary.write_text("fixture binary\n")
        installed_skill = smoke.skill_path(self.project, "gemini")
        installed_skill.parent.mkdir(parents=True)
        installed_skill.write_text("---\nname: agentarch-discover-product\n---\nFixture skill\n")
        args = ["--host", "gemini", "--host-binary", str(binary), "--model", "gemini-3.5-flash",
                "--owner", "Test Owner", "--contact", "Test Contact", "--agentarch-bin", str(binary), "--output-dir", str(output)]
        def setup(*unused):
            return self.project, {}, os.environ.copy()
        with mock.patch.object(smoke, "executable", side_effect=lambda value: value), \
             mock.patch.object(smoke, "run", return_value=(process(), "0.68.0", "PRIVATE_TRANSCRIPT")), \
             mock.patch.object(smoke, "setup_fixture", side_effect=setup), \
             mock.patch.object(smoke, "discover", return_value={"mcp": "connected", "skills": "discovered"}), \
             mock.patch.object(smoke, "auth_status", return_value={"status": "missing"}), \
             mock.patch.object(smoke, "host_command") as host_command:
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(smoke.main(args), 4)
            host_command.assert_not_called()
        report = json.loads((output / "report.json").read_text())
        self.assertEqual(report["status"], "blocked")
        self.assertEqual(report["reason"], "authentication_missing_or_unavailable")
        self.assertFalse(report["model_attempted"])
        self.assertNotIn("PRIVATE", json.dumps(report))
        self.assertEqual(sorted(path.name for path in output.iterdir()), ["report.json"])

    def test_bounded_process_timeout_and_output_limit(self):
        metadata, _, _ = smoke.run([sys.executable, "-c", "import time;time.sleep(20)"], self.project, timeout=.1)
        self.assertTrue(metadata["timed_out"])
        with mock.patch.object(smoke, "MAX_BYTES", 100):
            metadata, _, _ = smoke.run([sys.executable, "-c", "print('x'*10000)"], self.project)
        self.assertTrue(metadata["output_limit"])
        self.assertEqual(list(self.project.iterdir()), [])

    def test_timeout_terminates_child_process_group(self):
        marker = self.project / "child-survived"
        child = "import time,pathlib;time.sleep(.5);pathlib.Path(" + repr(str(marker)) + ").write_text('unexpected')"
        parent = "import subprocess,sys,time;subprocess.Popen([sys.executable,'-c'," + repr(child) + "]);time.sleep(20)"
        metadata, _, _ = smoke.run([sys.executable, "-c", parent], self.project, timeout=.1)
        self.assertTrue(metadata["timed_out"])
        time.sleep(.6)
        self.assertFalse(marker.exists())


if __name__ == "__main__":
    unittest.main()
