"""The real graph preserves the exact approval state produced by the shared engine."""
from app.agent import native, runner
from app.agent.principal import Principal
from app.infra.provider import ModelResponse, ToolUseBlock, TextBlock


def test_graph_preserves_pending_call_and_resumes_the_shared_engine(monkeypatch):
    calls = []
    def provider(**kwargs):
        calls.append(kwargs)
        if len(calls) == 1:
            return ModelResponse([ToolUseBlock("message-1", "notify_customer", {"conversation_id": "conv-1", "body": "hello"})])
        assert kwargs["messages"][-1]["content"][0]["tool_use_id"] == "message-1"
        return ModelResponse([TextBlock("[ESCALATE] messaging integration unavailable")])
    monkeypatch.setattr(native, "create_message", provider)
    principal = Principal("acme", "operator", "cus-42", role="support_supervisor")
    first = runner.run("Send a customer update", principal)
    assert first.status == "awaiting_approval"
    assert first.state["steps"] == 1 and len(calls) == 1
    final = runner.run("Send a customer update", principal, resume=first.state,
                       approved=runner.grant_for_state(first.state, principal))
    assert final.status == "escalated"
    assert final.steps == 2 and final.tool_calls == 1 and len(calls) == 2
