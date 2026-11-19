from dataclasses import asdict
import copy
import inspect

from app.agent.execution import run_loop, grant_for_state
from app.agent.principal import Principal
from app.agent.authorization import ApprovalGrant
from app.agent.tools import ApprovalRequired
from app.infra.provider import ModelResponse, ToolUseBlock, TextBlock, Usage


def principal():
    values = dict(tenant_id="acme", subject="original", role="account_manager")
    if "customer_id" in inspect.signature(Principal).parameters:
        values["customer_id"] = "cus-42"
    return Principal(**values)


def test_resume_executes_exact_pending_call_before_model_and_keeps_batch_and_limits():
    p = principal()
    manifest = {"id": "test", "_system_prompt": "test", "autonomy": {"level": "L2_act_reversible",
        "max_steps": 2, "max_tool_calls": 3, "budget": {"usd_per_run": 1, "tokens_per_run": 1000}},
        "model": {"id": "claude-sonnet-4-5-20250929", "provider": "anthropic", "params": {"temperature": 0, "max_output_tokens": 50}}}
    tools = {name: {"id": name, "description_for_model": name, "input_schema": {"type": "object"},
        "effect": "read" if name == "lookup" else "irreversible", "approval": {"approver_role": "account_manager"}}
        for name in ["lookup", "close"]}
    events = []
    def provider(**kwargs):
        events.append("model")
        if events.count("model") == 1:
            return ModelResponse([ToolUseBlock("r", "lookup", {}), ToolUseBlock("c", "close", {"account_id": "a"}), ToolUseBlock("d", "close", {"account_id": "b"})], Usage(2, 2))
        assert events == ["model", "lookup", "close:a", "close:b", "model"]
        assert len(kwargs["messages"][-1]["content"]) == 3
        return ModelResponse([TextBlock("Completed")], Usage(2, 2))
    def dispatch(name, args, spec, p, approved=None, call_id=""):
        if name == "close":
            if not isinstance(approved, ApprovalGrant) or not approved.consume(call_id, name, args, spec, p):
                raise ApprovalRequired(name, args, spec)
            events.append("close:"+args["account_id"])
        else: events.append(name)
        return {"completed": True}
    def run(resume=None, approved=None):
        return run_loop("request", p, manifest=manifest, tools=tools, create_message=provider, dispatch=dispatch,
            input_guardrail=lambda q: (True, ""), output_guardrail=lambda text, citations: (True, ""),
            render_for_approver=lambda spec,args,p: {"arguments":args,"approver_role":"account_manager"}, resume=resume, approved=approved)
    first = run()
    assert first.status == "awaiting_approval" and first.state["steps"] == 1
    assert events == ["model", "lookup"]
    second = run(first.state, grant_for_state(first.state, p))
    assert second.status == "awaiting_approval" and second.state["pending_calls"][0]["id"] == "d"
    assert events == ["model", "lookup", "close:a"]
    final = run(second.state, grant_for_state(second.state, p))
    assert final.status == "answered" and final.steps == 2 and final.tool_calls == 3 and final.tokens == 8
    assert final.actions == ["lookup", "close", "close"]


def test_resume_with_changed_requester_or_manifest_never_executes():
    # Covered using the public engine in the batch test; this assertion exercises state binding
    # before any provider/tool call even when an attacker has an otherwise well-shaped state.
    p = principal()
    manifest={"autonomy":{"budget":{}}, "id":"now"}
    state={"principal":asdict(p),"contract_hash":"stale","tool_calls":0,"spent_usd":0,"steps":1,"tokens":0,"actions":[]}
    outcome=run_loop("request",p,manifest=manifest,tools={},create_message=lambda **kw: (_ for _ in ()).throw(AssertionError()),
        dispatch=None,input_guardrail=None,output_guardrail=None,render_for_approver=None,resume=state)
    assert outcome.status == "escalated"
