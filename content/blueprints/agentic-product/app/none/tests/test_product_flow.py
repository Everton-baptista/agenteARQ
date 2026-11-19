"""Exercise the generated HTTP service through a real tool, approval and subsequent turn."""
from starlette.testclient import TestClient

from app.agent import runner
from app.agent.memory import Memory
from app.agent.principal import Principal
from app.api import approvals, deps
from app.api.main import create_app
from app.domain import accounts
from app.infra.provider import ModelResponse, TextBlock, ToolUseBlock, Usage
from app.infra.store import InMemory


def test_approval_preserves_requester_checks_role_and_performs_the_exact_action(monkeypatch):
    monkeypatch.setattr(accounts, "_accounts", {("acme", "acct-4471"): {"account_id": "acct-4471", "status": "active"}})
    monkeypatch.setattr(approvals, "queue", approvals.Queue(InMemory()))
    monkeypatch.setitem(deps._DEMO_TOKENS, "second-manager", Principal("acme", "different-approver", "account_manager"))
    calls = []
    def provider(**kwargs):
        calls.append(kwargs)
        if len(calls) == 1:
            return ModelResponse([ToolUseBlock("close-1", "close_account", {"account_id": "acct-4471"})], Usage(2,2))
        assert accounts.lookup("acct-4471", "acme")["status"] == "closed"
        assert kwargs["messages"][-1]["content"][0]["tool_use_id"] == "close-1"
        return ModelResponse([TextBlock("Account closed")], Usage(2,2))
    monkeypatch.setattr(runner, "create_message", provider)
    with TestClient(create_app()) as client:
        first = client.post("/v1/ask", json={"instruction": "close acct-4471"}, headers={"Authorization": "Bearer demo-token-manager"})
        assert first.status_code == 200
        pending = first.json()
        assert pending["status"] == "awaiting_approval" and len(calls) == 1
        assert accounts.lookup("acct-4471", "acme")["status"] == "active"
        url = "/v1/approvals/" + pending["approval_id"]
        denied = client.post(url, json={"decision": "approve"}, headers={"Authorization": "Bearer demo-token-readonly"})
        assert denied.status_code == 404
        completed = client.post(url, json={"decision": "approve"}, headers={"Authorization": "Bearer second-manager"})
        assert completed.status_code == 200 and completed.json()["status"] == "answered"
        assert completed.json()["actions"] == ["close_account"]
        assert len(calls) == 2
        assert client.post(url, json={"decision": "approve"}, headers={"Authorization": "Bearer second-manager"}).status_code == 404


def test_successful_tool_reference_is_recalled_on_the_next_http_turn(monkeypatch):
    runner._memory.cache_clear()
    prompts = []
    responses = iter([ModelResponse([ToolUseBlock("lookup-1", "lookup_account", {"account_id": "acct-4471"})]),
                      ModelResponse([TextBlock("Found account")]), ModelResponse([TextBlock("Previous action recalled")])])
    def provider(**kwargs):
        prompts.append(kwargs)
        return next(responses)
    monkeypatch.setattr(runner, "create_message", provider)
    with TestClient(create_app()) as client:
        headers = {"Authorization": "Bearer demo-token-manager"}
        assert client.post("/v1/ask", json={"instruction": "look up acct-4471"}, headers=headers).json()["status"] == "answered"
        assert client.post("/v1/ask", json={"instruction": "what was the previous action?"}, headers=headers).json()["status"] == "answered"
    assert "lookup_account" in prompts[-1]["messages"][0]["content"]
    assert "remembered_facts" in prompts[-1]["messages"][0]["content"]
    assert "remembered_facts" not in prompts[-1]["system"]


def test_memory_is_isolated_between_subjects_in_the_same_tenant():
    memory = Memory(InMemory())
    first = Principal("acme", "one", "account_manager")
    second = Principal("acme", "two", "account_manager")
    memory.remember(first, "default", "last_action", "lookup_account")
    assert memory.recall(first, "default")
    assert memory.recall(second, "default") == []
