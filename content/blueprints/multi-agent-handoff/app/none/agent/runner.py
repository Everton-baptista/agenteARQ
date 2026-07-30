"""One agent's turn. Just the loop.

The dispatcher is in tools.py and the delegation boundary is in handoff.py, deliberately. Interleaving
all three is how a multi-agent system ends up with the authority check in one place, the budget check
in another, and a code path that misses one — and the termination properties become untestable without
a model, which means untested.

What this file is responsible for: call the model, count the step against the shared budget, hand tool
calls to the dispatcher, hand delegation to handoff, and stop. Every bound it enforces comes from the
manifest or from the shared Budget; none is written here.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

from ..infra import telemetry
from ..infra.provider import create_message
from .budget import Budget
from .guardrails import input_guardrail, output_guardrail
from .handoff import hand_off
from .manifest import ENTRY_AGENT, load_manifest, load_tools
from .principal import Principal
from .tools import ApprovalRequired, dispatch

# The handoff tool is not in any .tool.yaml, because it is not a capability the agent has over the
# world — it is the system's own control flow. Its schema is built from the caller's declared
# contracts, so an agent can only ever propose a delegation the manifest already permits.
HANDOFF_TOOL = "hand_off_to"


@dataclass
class Outcome:
    status: str                       # "answered" | "escalated" | "awaiting_approval"
    text: str = ""
    approval: dict | None = None
    state: dict | None = None
    tool_calls: int = 0
    cost_usd: float = 0.0
    # What the request actually did across every agent. Returned because a multi-agent system that
    # reports only its answer gives an operator no way to see that one request quietly used nine
    # steps across three agents.
    budget: dict = field(default_factory=dict)


def _tool_schemas(manifest: dict, tools: dict[str, dict]) -> list[dict]:
    schemas = [
        {"name": s["id"], "description": s["description_for_model"], "input_schema": s["input_schema"]}
        for s in tools.values()
    ]
    targets = [c["agent_id"] for c in manifest.get("handoff", {}).get("hands_off_to", [])]
    if targets:
        schemas.append(
            {
                "name": HANDOFF_TOOL,
                "description": (
                    "Delegate to a specialist. The target must be one of the declared agents; the "
                    "payload must match that agent's contract."
                ),
                "input_schema": {
                    "type": "object",
                    # An enum, not a free string. The model cannot name an agent that is not in the
                    # manifest, so the delegation graph a review can read is the delegation graph
                    # that exists.
                    "properties": {
                        "agent_id": {"type": "string", "enum": targets},
                        "payload": {"type": "object"},
                    },
                    "required": ["agent_id", "payload"],
                },
            }
        )
    return schemas


def run_agent(
    manifest: dict,
    task: str,
    budget: Budget,
    principal: Principal,
    authority: str = "full",
    resume: dict | None = None,
    approved: set[str] | None = None,
) -> Outcome:
    """One agent, bounded by the shared budget rather than by its own.

    `authority` is what a handoff granted. It only narrows: an agent delegated `read_only` may not
    write even where its own manifest allows it, because otherwise the contract is advisory.
    """
    tools = load_tools(manifest)
    agent_id = manifest["id"]

    messages = resume["messages"] if resume else [
        {"role": "user", "content": f"<task>\n{task}\n</task>"}
    ]
    calls = (resume or {}).get("tool_calls", 0)

    for _ in range(manifest["autonomy"]["max_steps"]):
        ok, reason = budget.spend_step(agent_id)
        if not ok:
            return Outcome(status="escalated", text=f"[ESCALATE] {reason}",
                           tool_calls=calls, budget=budget.snapshot(), cost_usd=budget.spent_usd)

        with telemetry.span(
            "agent.model_call",
            **{
                "gen_ai.request.model": manifest["model"]["id"],
                "agent.id": agent_id,
                "agent.authority": authority,
                "agent.depth": budget.depth,
                "tenant": principal.tenant_id,
            },
        ), telemetry.Timer() as timer:
            response = create_message(
                model=manifest["model"]["id"],
                max_tokens=manifest["model"]["params"]["max_output_tokens"],
                system=manifest["_system_prompt"],
                tools=_tool_schemas(manifest, tools),
                messages=messages,
            )

        cost = telemetry.record_model_call(
            model=manifest["model"]["id"],
            tenant=principal.tenant_id,
            input_tokens=response.usage.input_tokens,
            output_tokens=response.usage.output_tokens,
            latency_ms=timer.ms,
        )
        ok, reason = budget.spend_usd(cost)
        if not ok:
            return Outcome(status="escalated", text=f"[ESCALATE] {reason}",
                           tool_calls=calls, budget=budget.snapshot(), cost_usd=budget.spent_usd)

        uses = [b for b in response.content if b.type == "tool_use"]
        if not uses:
            text = "".join(b.text for b in response.content if b.type == "text")
            ok, reason = output_guardrail(text)
            telemetry.record_guardrail(
                "output", "allow" if ok else "block", "control.ai.privacy.pii_leakage"
            )
            status = "answered" if ok else "escalated"
            return Outcome(
                status=status,
                text=text if ok else f"[ESCALATE] {reason}",
                tool_calls=calls,
                budget=budget.snapshot(),
                cost_usd=budget.spent_usd,
            )

        calls += len(uses)
        if calls > manifest["autonomy"]["max_tool_calls"]:
            return Outcome(status="escalated", text="[ESCALATE] tool call budget exhausted",
                           tool_calls=calls, budget=budget.snapshot(), cost_usd=budget.spent_usd)

        messages.append({"role": "assistant", "content": response.content})
        results = []
        for use in uses:
            if use.name == HANDOFF_TOOL:
                out = hand_off(
                    caller=manifest,
                    to_agent=use.input["agent_id"],
                    payload=use.input["payload"],
                    budget=budget,
                    principal=principal,
                    run_agent=_delegate,
                )
            else:
                spec = tools.get(use.name)
                try:
                    with telemetry.span(
                        "agent.tool_call",
                        **{"gen_ai.tool.name": use.name, "agent.id": agent_id,
                           "agent.tool.effect": (spec or {}).get("effect", "unknown")},
                    ):
                        out = dispatch(use.name, use.input, spec, principal, authority, approved)
                    telemetry.record_guardrail(
                        "action", "block" if "error" in out else "allow",
                        "control.ai.tool.least_privilege",
                    )
                except ApprovalRequired as pause:
                    telemetry.record_guardrail(
                        "action", "await_approval",
                        "control.ai.tool.irreversible_requires_approval",
                    )
                    from .guardrails import action_guardrail  # noqa: F401  (kept close to its use)

                    return Outcome(
                        status="awaiting_approval",
                        approval={
                            "tool": pause.spec["id"],
                            "effect": pause.spec["effect"],
                            "reversible": False,
                            "requested_by_agent": agent_id,
                            "delegated_authority": authority,
                            "approver_role": pause.spec.get("approval", {}).get("approver_role", "unknown"),
                            "on_timeout": pause.spec.get("approval", {}).get("on_timeout", "deny"),
                            "arguments": pause.args,
                        },
                        state={
                            "agent_id": agent_id,
                            "messages": _serialisable(messages),
                            "task": task,
                            "tool_calls": calls,
                            "tool": pause.tool,
                            "authority": authority,
                            "budget": budget.snapshot(),
                        },
                        tool_calls=calls,
                        budget=budget.snapshot(),
                        cost_usd=budget.spent_usd,
                    )
            results.append(
                {
                    "type": "tool_result",
                    "tool_use_id": use.id,
                    "content": json.dumps(out),
                    "is_error": "error" in out,
                }
            )
        messages.append({"role": "user", "content": results})

    return Outcome(status="escalated", text="[ESCALATE] step budget exhausted",
                   tool_calls=calls, budget=budget.snapshot(), cost_usd=budget.spent_usd)


def _delegate(manifest: dict, task: str, budget: Budget, principal: Principal, authority: str) -> str:
    """What handoff calls. Returns text, because a specialist's reply is data to its caller.

    Returning an Outcome would let a specialist's status leak upward and be mistaken for the system's
    — a specialist that escalated has not necessarily ended the request, and the orchestrator is the
    one that decides.
    """
    outcome = run_agent(manifest, task, budget, principal, authority=authority)
    return outcome.text


def run(
    question: str,
    principal: Principal,
    resume: dict | None = None,
    approved: set[str] | None = None,
) -> Outcome:
    """The entry point. One shared budget per request, created here and passed all the way down."""
    ok, reason = input_guardrail(question)
    telemetry.record_guardrail(
        "input", "allow" if ok else "block", "control.ai.genai.prompt_injection"
    )
    if not ok:
        return Outcome(status="escalated", text=f"[ESCALATE] {reason}")

    entry = load_manifest(ENTRY_AGENT)
    autonomy = entry["autonomy"]

    budget = Budget(
        # The system's bound, not one agent's. Three agents with ten steps each is a system with
        # thirty, and a cycle between them is a system with no bound at all.
        max_steps=autonomy["max_steps"],
        max_depth=entry.get("handoff", {}).get("max_depth", 3),
        max_usd=autonomy["budget"]["usd_per_run"],
    )
    if resume and resume.get("budget"):
        snapshot = resume["budget"]
        budget.used = snapshot.get("steps_used", 0)
        budget.spent_usd = snapshot.get("spent_usd", 0.0)

    budget.enter(ENTRY_AGENT)
    with telemetry.span("agent.run", **{"agent.id": ENTRY_AGENT, "tenant": principal.tenant_id}):
        target = resume.get("agent_id", ENTRY_AGENT) if resume else ENTRY_AGENT
        manifest = entry if target == ENTRY_AGENT else load_manifest(target)
        return run_agent(
            manifest,
            question,
            budget,
            principal,
            authority=(resume or {}).get("authority", "full"),
            resume=resume,
            approved=approved,
        )


def _serialisable(messages: list) -> list:
    out = []
    for m in messages:
        content = m["content"]
        if isinstance(content, str):
            out.append({"role": m["role"], "content": content})
            continue
        out.append(
            {
                "role": m["role"],
                "content": [
                    b if isinstance(b, dict) else (b.model_dump() if hasattr(b, "model_dump") else dict(b))
                    for b in content
                ],
            }
        )
    return out
