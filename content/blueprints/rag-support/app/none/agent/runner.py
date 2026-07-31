"""The agent loop, bounded by the manifest.

This module is the reason the layout is what it is: it imports the provider client, the domain,
and the guardrails — and nothing from app/api. There is no `Request`, no header, no framework
here. Run it from the service, a queue worker, a test or a REPL and it behaves identically.

`agentarch check` enforces that direction as control.ai.api.core_transport_separated. It is worth
stating why it is a control rather than a convention: once a route handler is importable from
here, the fastest fix to any problem is to reach for the request object, and within a few weeks
the agent can only run inside a web server.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

from ..infra import telemetry
from ..infra.provider import create_message
from .guardrails import input_guardrail, output_guardrail, render_for_approver
from .manifest import load_manifest, load_tools
from .principal import Principal
from .retrieval import render_untrusted, retrieve
from .tools import ApprovalRequired, dispatch


@dataclass
class Outcome:
    """What one turn produced.

    Three shapes, and the caller has to handle all three: an answer, an escalation, or a pause
    waiting on a person. Collapsing the third into an error is how approval turns into a failed
    request that the caller retries until it succeeds.
    """

    status: str                       # "answered" | "escalated" | "awaiting_approval"
    text: str = ""
    approval: dict | None = None      # what to show the approver
    state: dict | None = None         # opaque conversation state, to resume with
    citations: list[str] = field(default_factory=list)
    tool_calls: int = 0
    cost_usd: float = 0.0


def _tool_schemas(tools: dict[str, dict]) -> list[dict]:
    return [
        {
            "name": s["id"],
            "description": s["description_for_model"],
            "input_schema": s["input_schema"],
        }
        for s in tools.values()
    ]


def run(
    question: str,
    principal: Principal,
    resume: dict | None = None,
    approved: set[str] | None = None,
) -> Outcome:
    """One turn, or the continuation of one that paused for approval.

    `resume` carries the message list from a paused run so that approving does not re-run the
    model from the top. Re-running would cost a second call and, worse, could produce a different
    tool call than the one the human actually approved.
    """
    manifest = load_manifest()
    tools = load_tools(manifest)

    ok, reason = input_guardrail(question)
    telemetry.record_guardrail(
        "input", "allow" if ok else "block", "control.ai.genai.prompt_injection"
    )
    if not ok:
        # Before any model call, so a rejected input costs nothing.
        return Outcome(status="escalated", text=f"[ESCALATE] {reason}")

    with telemetry.span("agent.retrieval", **{"rag.top_k": manifest["context"]["rag"]["top_k"]}) as s:
        passages = retrieve(question, top_k=manifest["context"]["rag"]["top_k"])
        # The count and the ids, never the passage text: a trace backend is a log aggregator with
        # better indexing, and retrieved content is somebody's data.
        s.set_attribute("rag.passages", len(passages))
    if resume:
        messages = resume["messages"]
        calls = resume.get("tool_calls", 0)
    else:
        messages = [{"role": "user", "content": render_untrusted(passages, question)}]
        calls = 0

    max_steps = manifest["autonomy"]["max_steps"]
    max_tool_calls = manifest["autonomy"]["max_tool_calls"]
    budget_usd = manifest["autonomy"]["budget"]["usd_per_run"]
    spent = resume.get("spent_usd", 0.0) if resume else 0.0

    for step in range(max_steps):
        with telemetry.span(
            "agent.model_call",
            **{
                "gen_ai.request.model": manifest["model"]["id"],
                "gen_ai.operation.name": "chat",
                "agent.step": step,
                "tenant": principal.tenant_id,
            },
        ), telemetry.Timer() as timer:
            response = create_message(
                # The provider is read from the contract and passed down, never read inside
                # infra — see the note at the top of infra/provider.py. It is the same field
                # `agentarch check` reads for control.ai.supply.model_pinned, so the manifest
                # and the call cannot name different providers.
                provider=manifest["model"]["provider"],
                model=manifest["model"]["id"],
                max_tokens=manifest["model"]["params"]["max_output_tokens"],
                temperature=manifest["model"]["params"]["temperature"],
                system=manifest["_system_prompt"],
                tools=_tool_schemas(tools),
                messages=messages,
            )

        spent += telemetry.record_model_call(
            model=manifest["model"]["id"],
            tenant=principal.tenant_id,
            input_tokens=response.usage.input_tokens,
            output_tokens=response.usage.output_tokens,
            latency_ms=timer.ms,
        )
        # The declared budget, enforced with the same figure that reaches the dashboard — so the
        # bill and the limit cannot disagree. Checked after the call because the cost of a call is
        # not known until it returns; the bound is on continuing, not on starting.
        if spent > budget_usd:
            return Outcome(
                status="escalated",
                text=f"[ESCALATE] run cost budget of ${budget_usd} exhausted",
                tool_calls=calls,
                cost_usd=spent,
            )

        tool_uses = [b for b in response.content if b.type == "tool_use"]
        if not tool_uses:
            text = "".join(b.text for b in response.content if b.type == "text")
            cited = [p["id"] for p in passages if p["id"] in text]
            ok, reason = output_guardrail(text, bool(cited))
            telemetry.record_guardrail(
                "output", "allow" if ok else "block", "control.ai.rag.citation_required"
            )
            if not ok:
                return Outcome(
                    status="escalated",
                    text=f"[ESCALATE] {reason}",
                    tool_calls=calls,
                    cost_usd=spent,
                )
            return Outcome(
                status="answered",
                text=text,
                citations=cited,
                tool_calls=calls,
                cost_usd=spent,
            )

        if calls + len(tool_uses) > max_tool_calls:
            return Outcome(
                status="escalated",
                text="[ESCALATE] tool call budget exhausted",
                tool_calls=calls,
                cost_usd=spent,
            )
        calls += len(tool_uses)

        messages.append({"role": "assistant", "content": response.content})
        results = []
        for use in tool_uses:
            spec = tools.get(use.name)
            try:
                with telemetry.span(
                    "agent.tool_call",
                    **{
                        "gen_ai.tool.name": use.name,
                        "agent.tool.effect": (spec or {}).get("effect", "unknown"),
                        "tenant": principal.tenant_id,
                    },
                ):
                    out = dispatch(use.name, use.input, spec, principal, approved)
                telemetry.record_guardrail(
                    "action",
                    "block" if "error" in out else "allow",
                    "control.ai.tool.least_privilege",
                )
            except ApprovalRequired as pause:
                telemetry.record_guardrail(
                    "action", "await_approval", "control.ai.tool.irreversible_requires_approval"
                )
                # The run stops here with nothing performed. Everything needed to continue is
                # returned to the transport layer, which is where a human can be reached.
                return Outcome(
                    status="awaiting_approval",
                    approval=render_for_approver(pause.spec, pause.args, principal),
                    cost_usd=spent,
                    state={
                        "messages": _serialisable(messages),
                        "question": question,
                        "tool_calls": calls,
                        "spent_usd": spent,
                        "tool": pause.tool,
                    },
                    tool_calls=calls,
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

    return Outcome(
        status="escalated",
        text="[ESCALATE] step budget exhausted",
        tool_calls=calls,
        cost_usd=spent,
    )


def _serialisable(messages: list) -> list:
    """Provider content blocks are objects; the pending store needs plain data.

    Kept deliberately small: a paused run holds customer text, so the less of it that is copied
    into another structure with its own lifetime, the fewer places retention has to be enforced.
    """
    out = []
    for m in messages:
        content = m["content"]
        if isinstance(content, str):
            out.append({"role": m["role"], "content": content})
            continue
        blocks = []
        for b in content:
            if isinstance(b, dict):
                blocks.append(b)
            else:
                blocks.append(b.model_dump() if hasattr(b, "model_dump") else dict(b))
        out.append({"role": m["role"], "content": blocks})
    return out
