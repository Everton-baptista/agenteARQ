"""One bounded execution engine shared by transport and framework adapters."""
from __future__ import annotations

import copy
import json
from dataclasses import asdict, dataclass, field

from ..infra import telemetry
from ..infra.provider import normalise
from .authorization import ApprovalGrant, digest
from .tools import ApprovalRequired


@dataclass
class Outcome:
    status: str
    text: str = ""
    approval: dict | None = None
    state: dict | None = None
    citations: list[str] = field(default_factory=list)
    actions: list[str] = field(default_factory=list)
    tool_calls: int = 0
    cost_usd: float = 0.0
    steps: int = 0
    tokens: int = 0
    budget: dict = field(default_factory=dict)


def run_loop(question, principal, *, manifest, tools, create_message, dispatch,
             input_guardrail, output_guardrail, render_for_approver,
             initial_content=None, passages=None, resume=None, approved=None,
             memory=None, conversation_id="default", shared_budget=None):
    """Resume a pending tool before asking the model anything else.

    Pending state contains the original batch, previously completed results and all counters.
    A single-use grant can authorize only its first pending call. New calls require new decisions.
    """
    autonomy = manifest["autonomy"]
    limits = autonomy["budget"]
    state = copy.deepcopy(resume) if resume else {
        "question": question, "principal": asdict(principal),
        "messages": [{"role": "user", "content": initial_content or f"<operator_request>\n{question}\n</operator_request>"}],
        "tool_calls": 0, "spent_usd": 0.0, "steps": 0, "tokens": 0,
        "pending_calls": [], "results": [], "actions": [], "seen_call_ids": [],
        "passages": passages or [], "conversation_id": conversation_id,
        "contract_hash": digest(manifest),
    }

    def outcome(status, text="", **extra):
        return Outcome(status, text=text, tool_calls=state["tool_calls"], cost_usd=state["spent_usd"],
                       steps=state["steps"], tokens=state["tokens"], actions=list(state["actions"]),
                       budget=shared_budget.snapshot() if shared_budget else {}, **extra)

    if resume and (state.get("principal") != asdict(principal) or state.get("contract_hash") != digest(manifest)):
        return outcome("escalated", "[ESCALATE] requester or reviewed contract changed; a new run is required")
    ok, reason = input_guardrail(question)
    telemetry.record_guardrail("input", "allow" if ok else "block", "control.ai.genai.input_guardrail")
    if not ok:
        return outcome("escalated", f"[ESCALATE] {reason}")
    try:
        telemetry.require_model_price(manifest["model"]["id"])
    except telemetry.UnknownModelPrice as err:
        return outcome("escalated", f"[ESCALATE] {err}")

    while True:
        # Finish the suspended batch, including already collected results, exactly once.
        while state["pending_calls"]:
            use = state["pending_calls"][0]
            spec = tools.get(use["name"])
            try:
                out = dispatch(use["name"], use["input"], spec, principal,
                               approved=approved, call_id=use["id"])
            except ApprovalRequired as pause:
                telemetry.record_guardrail("action", "await_approval", "control.ai.tool.irreversible_requires_approval")
                preview = render_for_approver(pause.spec, pause.arguments, principal)
                preview["call_id"] = use["id"]
                preview["arguments_hash"] = digest(use["input"])
                if shared_budget:
                    state["budget"] = shared_budget.snapshot()
                state["agent_id"] = manifest["id"]
                state["tool"] = use["name"]
                state["approval_spec"] = pause.spec
                return outcome("awaiting_approval", approval=preview, state=copy.deepcopy(state))
            except Exception:
                # A failed side-effect is not retried by the model loop. Its outcome may be uncertain.
                return outcome("escalated", "[ESCALATE] tool execution failed; reconcile its effect before starting another run")
            telemetry.record_guardrail("action", "block" if isinstance(out, dict) and "error" in out else "allow", "control.ai.tool.action_guardrail")
            if not isinstance(out, dict):
                return outcome("escalated", "[ESCALATE] tool returned an invalid result")
            if "error" not in out and not out.get("simulated"):
                state["actions"].append(use["name"])
                if memory is not None:
                    # Store a narrow action reference, never request/response bodies or email values.
                    memory.remember(principal, state["conversation_id"], "last_action", use["name"])
            state["results"].append({"type": "tool_result", "tool_use_id": use["id"],
                                     "content": json.dumps(out), "is_error": "error" in out})
            state["pending_calls"].pop(0)
            approved = None
        if state["results"]:
            state["messages"].append({"role": "user", "content": state["results"]})
            state["results"] = []

        if state["steps"] >= autonomy["max_steps"]:
            return outcome("escalated", "[ESCALATE] step budget exhausted")
        if state["spent_usd"] >= limits["usd_per_run"]:
            return outcome("escalated", "[ESCALATE] run cost budget exhausted")
        if state["tokens"] >= limits.get("tokens_per_run", 2**63):
            return outcome("escalated", "[ESCALATE] token budget exhausted")
        if shared_budget:
            ok, reason = shared_budget.spend_step(manifest["id"])
            if not ok:
                return outcome("escalated", f"[ESCALATE] {reason}")
        state["steps"] += 1
        with telemetry.span("agent.model_call", **{"gen_ai.request.model": manifest["model"]["id"], "agent.step": state["steps"]}):
            with telemetry.Timer() as timer:
                response = create_message(provider=manifest["model"]["provider"], model=manifest["model"]["id"],
                    max_tokens=min(manifest["model"]["params"]["max_output_tokens"], limits.get("tokens_per_run", 2**63)-state["tokens"]),
                    temperature=manifest["model"]["params"]["temperature"], system=manifest["_system_prompt"],
                    tools=[{"name": s["id"], "description": s["description_for_model"], "input_schema": s["input_schema"]} for s in tools.values()],
                    messages=state["messages"])
            cost = telemetry.record_model_call(model=manifest["model"]["id"], tenant=principal.tenant_id,
                input_tokens=response.usage.input_tokens, output_tokens=response.usage.output_tokens, latency_ms=timer.ms)
        state["spent_usd"] += cost
        state["tokens"] += response.usage.input_tokens + response.usage.output_tokens
        if shared_budget:
            ok, reason = shared_budget.spend_tokens(response.usage.input_tokens + response.usage.output_tokens)
            if not ok:
                return outcome("escalated", f"[ESCALATE] {reason}")
            ok, reason = shared_budget.spend_usd(cost)
            if not ok:
                return outcome("escalated", f"[ESCALATE] {reason}")
        if state["spent_usd"] > limits["usd_per_run"] or state["tokens"] > limits.get("tokens_per_run", 2**63):
            return outcome("escalated", "[ESCALATE] run cost or token budget exhausted")
        blocks = normalise([{"role": "assistant", "content": response.content}])[0]["content"]
        uses = [b for b in blocks if b.get("type") == "tool_use"]
        if not uses:
            text = "".join(b.get("text", "") for b in blocks if b.get("type") == "text")
            cited = [p["id"] for p in state["passages"] if p["id"] in text]
            ok, reason = output_guardrail(text, cited)
            if not text.strip() or text.lstrip().startswith("[ESCALATE]"):
                ok, reason = False, "no completed answer"
            telemetry.record_guardrail("output", "allow" if ok else "block", "control.ai.genai.output_guardrail")
            return outcome("answered" if ok else "escalated", text if ok else f"[ESCALATE] {reason}", citations=cited)
        ids = [u.get("id") for u in uses]
        if any(not i or i in state["seen_call_ids"] for i in ids) or len(set(ids)) != len(ids):
            return outcome("escalated", "[ESCALATE] duplicate or missing tool call id")
        if state["tool_calls"] + len(uses) > autonomy["max_tool_calls"]:
            return outcome("escalated", "[ESCALATE] tool call budget exhausted")
        if shared_budget:
            ok, reason = shared_budget.spend_tools(len(uses))
            if not ok:
                return outcome("escalated", f"[ESCALATE] {reason}")
        state["tool_calls"] += len(uses)
        state["seen_call_ids"].extend(ids)
        state["messages"].append({"role": "assistant", "content": blocks})
        state["pending_calls"] = uses


def grant_for_state(state: dict, principal) -> ApprovalGrant:
    return ApprovalGrant.for_call(state["pending_calls"][0], state["approval_spec"], principal)
