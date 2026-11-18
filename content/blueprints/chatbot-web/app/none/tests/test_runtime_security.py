"""Regressions at the boundaries where a plausible declaration can hide a failed control."""
from concurrent.futures import ThreadPoolExecutor
from dataclasses import asdict
import importlib.util
import inspect
from pathlib import Path
import sys
import types

import pytest

from app.agent.authorization import ApprovalGrant, validate_arguments
from app.agent.principal import Principal
from app.api.approvals import Pending, Queue
from app.infra.store import InMemory
from app.infra import telemetry


def principal(subject="requester"):
    fields = inspect.signature(Principal).parameters
    values = dict(tenant_id="acme", subject=subject, role="account_manager")
    if "customer_id" in fields:
        values["customer_id"] = "cus-42"
    return Principal(**values)


def spec():
    return {"id": "close_account", "effect": "irreversible", "_approval": "human",
        "description_for_model": "Close an account", "input_schema": {"type": "object",
            "required": ["account_id"], "additionalProperties": False,
            "properties": {"account_id": {"type": "string", "minLength": 1}}},
        "approval": {"approver_role": "account_manager", "timeout_s": 900, "on_timeout": "deny"}}


def test_tool_schema_rejects_missing_wrong_type_extra_and_remote_refs():
    for args in ({}, {"account_id": 12}, {"account_id": "a", "tenant_id": "other"}):
        assert not validate_arguments(args, spec())[0]
    assert validate_arguments({"account_id": "a"}, spec())[0]
    assert not validate_arguments({}, {"input_schema": {"$ref": "https://untrusted.invalid/schema"}})[0]
    assert not validate_arguments({}, {})[0]


def test_approval_cannot_change_arguments_principal_or_contract_and_is_single_use():
    p = principal()
    call = {"id": "call-1", "name": "close_account", "input": {"account_id": "a"}}
    grant = ApprovalGrant.for_call(call, spec(), p)
    assert not grant.consume("call-2", call["name"], call["input"], spec(), p)
    assert not grant.consume("call-1", call["name"], {"account_id": "b"}, spec(), p)
    assert not grant.consume("call-1", call["name"], call["input"], spec(), principal("another"))
    assert not grant.consume("call-1", call["name"], call["input"], {**spec(), "effect": "read"}, p)
    assert grant.consume("call-1", call["name"], call["input"], spec(), p)
    assert not grant.consume("call-1", call["name"], call["input"], spec(), p)


def test_concurrent_approval_is_consumed_once_and_unauthorized_role_cannot_consume():
    queue = Queue(InMemory())
    item = Pending("acme", "requester", "close_account", {"approver_role": "account_manager"}, {})
    id_ = queue.put(item)
    assert queue.take(id_, "acme", "approve", "viewer", role="viewer") is None
    with ThreadPoolExecutor(max_workers=8) as pool:
        results = list(pool.map(lambda _: queue.take(id_, "acme", "approve", "manager", role="account_manager"), range(20)))
    assert sum(item is not None for item in results) == 1


def test_unknown_price_cannot_disable_the_cost_budget():
    with pytest.raises(telemetry.UnknownModelPrice):
        telemetry.record_model_call(model="unconfigured", tenant="acme", input_tokens=1, output_tokens=1, latency_ms=0)


def test_production_refuses_demo_adapters(monkeypatch):
    from app.api.main import create_app
    monkeypatch.setenv("ENV", "production")
    with pytest.raises(RuntimeError, match="production"):
        create_app()


def _evaluation():
    path = Path("evals/run.py")
    module_spec = importlib.util.spec_from_file_location("runtime_eval_test", path)
    module = importlib.util.module_from_spec(module_spec)
    sys.modules[module_spec.name] = module
    module_spec.loader.exec_module(module)
    return module


@pytest.mark.parametrize("status", ["escalated", "awaiting_approval"])
def test_evaluation_never_upgrades_an_incomplete_runtime_outcome(monkeypatch, status):
    from app.agent import runner
    evaluation = _evaluation()
    monkeypatch.setattr(runner, "run", lambda *a, **kw: types.SimpleNamespace(status=status, text="", actions=[], citations=[]))
    result = evaluation.run_case(evaluation.Case("case", "Where is my order?", "answer"), principal(), False)
    assert result.status == status
    metric = evaluation.compute_metrics([result], {"task_success": {"kind": "deterministic", "threshold": 1, "direction": "min"}})[0]
    assert metric["value"] == 0


def test_evaluation_requires_declared_action_evidence(monkeypatch):
    from app.agent import runner
    evaluation = _evaluation()
    monkeypatch.setattr(runner, "run", lambda *a, **kw: types.SimpleNamespace(status="answered", text="Done", actions=[], citations=[]))
    case = evaluation.Case("case", "Perform the action", "answer", expected_tools=["close_account"])
    assert evaluation.run_case(case, principal(), False).status != "answered"


def test_metadata_logging_preserves_audit_fields_and_redacts_external_exception_messages():
    import json, logging
    from app.infra.logging_setup import MetadataFormatter
    record = logging.LogRecord("agent.approval.audit", logging.INFO, "", 0, "approval decision", (), None)
    record.decision, record.action_hash = "approve", "sha256-reviewed-operation"
    entry = json.loads(MetadataFormatter().format(record))
    assert entry["decision"] == "approve" and entry["action_hash"] == "sha256-reviewed-operation"
    external = logging.LogRecord("provider.sdk", logging.ERROR, "", 0, "secret customer prompt and sk-private", (), None)
    assert "sk-private" not in MetadataFormatter().format(external)
