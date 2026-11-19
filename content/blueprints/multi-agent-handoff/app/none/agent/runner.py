"""Bounded delegation over the same resumable execution engine as the single-agent service."""
from ..infra.provider import create_message
from .authorization import validate_arguments
from .budget import Budget
from .execution import Outcome, run_loop, grant_for_state
from .guardrails import input_guardrail, output_guardrail
from .handoff import hand_off, HandoffRefused
from .manifest import ENTRY_AGENT, load_manifest, load_tools
from .tools import dispatch

HANDOFF_TOOL = "hand_off_to"


def run_agent(manifest, task, budget, principal, authority="full", resume=None, approved=None):
    tools = load_tools(manifest)
    targets = [c["agent_id"] for c in manifest.get("handoff", {}).get("hands_off_to", [])]
    if targets:
        tools[HANDOFF_TOOL] = {"id": HANDOFF_TOOL, "effect": "read", "description_for_model": "Delegate a read-only analysis to a declared specialist.",
            "input_schema": {"type": "object", "required": ["agent_id", "payload"], "additionalProperties": False,
                "properties": {"agent_id": {"type": "string", "enum": targets}, "payload": {"type": "object"}}}}
    def dispatch_call(name, args, spec, p, approved=None, call_id=""):
        if name == HANDOFF_TOOL:
            valid, reason = validate_arguments(args, spec or {})
            if not valid:
                return {"error": reason, "retryable": False}
            # The shipped delegation contracts are read-only. An unimplemented authority path
            # must fail closed rather than dropping a child's pending human decision.
            contract = next(c for c in manifest["handoff"]["hands_off_to"] if c["agent_id"] == args["agent_id"])
            if contract["authority"] != "read_only":
                return {"error": "nested action approvals are not supported; delegate read_only", "retryable": False}
            return hand_off(manifest, args["agent_id"], args["payload"], budget, p, _delegate)
        return dispatch(name, args, spec, p, authority, approved=approved, call_id=call_id)
    def preview(spec, args, p):
        return {"tool": spec["id"], "effect": spec["effect"], "arguments": dict(args),
            "approver_role": spec["approval"]["approver_role"], "timeout_s": spec["approval"]["timeout_s"],
            "on_timeout": "deny", "requested_by_agent": manifest["id"]}
    result = run_loop(task, principal, manifest=manifest, tools=tools, create_message=create_message,
        dispatch=dispatch_call, input_guardrail=input_guardrail,
        output_guardrail=lambda text, cited: output_guardrail(text), render_for_approver=preview,
        resume=resume, approved=approved, shared_budget=budget)
    if result.state:
        result.state["authority"] = authority
    return result


def _delegate(manifest, task, budget, principal, authority):
    result = run_agent(manifest, task, budget, principal, authority)
    if result.status != "answered":
        raise HandoffRefused("specialist did not complete its task: " + result.status)
    return result.text


def run(question, principal, resume=None, approved=None):
    entry = load_manifest(ENTRY_AGENT)
    autonomy = entry["autonomy"]
    budget = Budget(max_steps=autonomy["max_steps"], max_depth=entry.get("handoff", {}).get("max_depth", 3),
        max_usd=autonomy["budget"]["usd_per_run"], max_tool_calls=autonomy["max_tool_calls"],
        max_tokens=autonomy["budget"].get("tokens_per_run", 40000))
    if resume:
        saved = resume.get("budget", {})
        budget.used = saved.get("steps_used", 0)
        budget.spent_usd = saved.get("spent_usd", 0)
        budget.tool_calls = saved.get("tool_calls", 0)
        budget.tokens_used = saved.get("tokens_used", 0)
    budget.enter(ENTRY_AGENT)
    return run_agent(entry, question, budget, principal, resume=resume, approved=approved)
