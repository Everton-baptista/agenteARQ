"""The agent loop as a LangGraph state machine.

This is the only file that differs from the `none` variant. Not the API, not the guardrails, not the
tools, not the domain, not secrets, storage, telemetry or resilience — one file. That is the claim
the layout makes, and it is demonstrated here rather than asserted.

What LangGraph buys is that the loop becomes a graph you can read: the guardrail points are nodes,
so "where is the output check" has a visual answer, and adding a node between two others cannot
accidentally skip one. What it does not buy is any of the properties in standard 08 — the guardrails
are the same functions, called from nodes, because a guardrail implemented as a framework feature is
a guardrail you lose when you change frameworks.

The same three outcomes come back: answered, escalated, awaiting_approval. The transport layer
cannot tell which variant it is running, which is the point.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Annotated, Any, TypedDict

from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import add_messages

from ..infra import telemetry
from ..infra.provider import create_message
from .guardrails import input_guardrail, output_guardrail, render_for_approver
from .manifest import load_manifest, load_tools
from .principal import Principal
from .retrieval import render_untrusted, retrieve
from .tools import ApprovalRequired, dispatch


@dataclass
class Outcome:
    """Identical to the `none` variant's Outcome, deliberately.

    The transport layer imports `runner.run` and handles three statuses. If this type differed, the
    framework choice would leak into app/api and the layers would no longer be swappable — which is
    the whole thing the layout is for.
    """

    status: str
    text: str = ""
    approval: dict | None = None
    state: dict | None = None
    citations: list[str] = field(default_factory=list)
    tool_calls: int = 0
    cost_usd: float = 0.0


class AgentState(TypedDict, total=False):
    messages: Annotated[list, add_messages]
    question: str
    principal: Any
    passages: list
    manifest: dict
    tools: dict
    outcome: Outcome
    calls: int
    spent: float
    approved: set


# ── nodes. Each guardrail point is one, so the graph shows where the checks are ──────────────

def check_input(state: AgentState) -> AgentState:
    ok, reason = input_guardrail(state["question"])
    telemetry.record_guardrail(
        "input", "allow" if ok else "block", "control.ai.genai.prompt_injection"
    )
    if not ok:
        # Before any model call, so a rejected input costs nothing.
        return {"outcome": Outcome(status="escalated", text=f"[ESCALATE] {reason}")}
    return {}


def do_retrieve(state: AgentState) -> AgentState:
    top_k = state["manifest"]["context"]["rag"]["top_k"]
    with telemetry.span("agent.retrieval", **{"rag.top_k": top_k}) as span:
        passages = retrieve(state["question"], top_k=top_k)
        # Counts and ids, never the passage text.
        span.set_attribute("rag.passages", len(passages))
    return {"passages": passages}


def build_context(state: AgentState) -> AgentState:
    """The untrusted block is built in its own node.

    Visible as a step rather than buried in a prompt string, because the moment retrieval and
    prompt-building are the same line of code, the concatenation invariant 2 forbids is one edit away.
    """
    if state.get("messages"):
        return {}
    return {"messages": [{"role": "user", "content": render_untrusted(state["passages"], state["question"])}]}


def call_model(state: AgentState) -> AgentState:
    manifest = state["manifest"]
    with telemetry.span(
        "agent.model_call",
        **{
            "gen_ai.request.model": manifest["model"]["id"],
            "gen_ai.operation.name": "chat",
            "tenant": state["principal"].tenant_id,
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
            tools=[
                {"name": s["id"], "description": s["description_for_model"], "input_schema": s["input_schema"]}
                for s in state["tools"].values()
            ],
            messages=state["messages"],
        )

    spent = state.get("spent", 0.0) + telemetry.record_model_call(
        model=manifest["model"]["id"],
        tenant=state["principal"].tenant_id,
        input_tokens=response.usage.input_tokens,
        output_tokens=response.usage.output_tokens,
        latency_ms=timer.ms,
    )
    if spent > manifest["autonomy"]["budget"]["usd_per_run"]:
        return {
            "spent": spent,
            "outcome": Outcome(
                status="escalated",
                text="[ESCALATE] run cost budget exhausted",
                tool_calls=state.get("calls", 0),
                cost_usd=spent,
            ),
        }
    return {"messages": [{"role": "assistant", "content": response.content}], "spent": spent}


def call_tools(state: AgentState) -> AgentState:
    last = state["messages"][-1]
    uses = [b for b in last.get("content", []) if getattr(b, "type", None) == "tool_use"]
    calls = state.get("calls", 0) + len(uses)
    if calls > state["manifest"]["autonomy"]["max_tool_calls"]:
        return {
            "calls": calls,
            "outcome": Outcome(
                status="escalated",
                text="[ESCALATE] tool call budget exhausted",
                tool_calls=calls,
                cost_usd=state.get("spent", 0.0),
            ),
        }

    results = []
    for use in uses:
        spec = state["tools"].get(use.name)
        try:
            with telemetry.span(
                "agent.tool_call",
                **{"gen_ai.tool.name": use.name, "agent.tool.effect": (spec or {}).get("effect", "unknown")},
            ):
                out = dispatch(use.name, use.input, spec, state["principal"], state.get("approved"))
            telemetry.record_guardrail(
                "action", "block" if "error" in out else "allow", "control.ai.tool.least_privilege"
            )
        except ApprovalRequired as pause:
            telemetry.record_guardrail(
                "action", "await_approval", "control.ai.tool.irreversible_requires_approval"
            )
            return {
                "calls": calls,
                "outcome": Outcome(
                    status="awaiting_approval",
                    approval=render_for_approver(pause.spec, pause.args, state["principal"]),
                    cost_usd=state.get("spent", 0.0),
                    tool_calls=calls,
                    state={
                        "messages": _serialisable(state["messages"]),
                        "question": state["question"],
                        "tool_calls": calls,
                        "spent_usd": state.get("spent", 0.0),
                        "tool": pause.tool,
                    },
                ),
            }
        results.append(
            {
                "type": "tool_result",
                "tool_use_id": use.id,
                "content": json.dumps(out),
                "is_error": "error" in out,
            }
        )
    return {"calls": calls, "messages": [{"role": "user", "content": results}]}


def check_output(state: AgentState) -> AgentState:
    last = state["messages"][-1]
    blocks = last.get("content", [])
    text = "".join(getattr(b, "text", "") for b in blocks if getattr(b, "type", None) == "text")
    cited = [p["id"] for p in state["passages"] if p["id"] in text]
    ok, reason = output_guardrail(text, bool(cited))
    telemetry.record_guardrail(
        "output", "allow" if ok else "block", "control.ai.rag.citation_required"
    )
    if not ok:
        return {
            "outcome": Outcome(
                status="escalated",
                text=f"[ESCALATE] {reason}",
                tool_calls=state.get("calls", 0),
                cost_usd=state.get("spent", 0.0),
            )
        }
    return {
        "outcome": Outcome(
            status="answered",
            text=text,
            citations=cited,
            tool_calls=state.get("calls", 0),
            cost_usd=state.get("spent", 0.0),
        )
    }


# ── edges ───────────────────────────────────────────────────────────────────────────────────

def after_input(state: AgentState) -> str:
    return END if state.get("outcome") else "retrieve"


def after_model(state: AgentState) -> str:
    if state.get("outcome"):
        return END
    last = state["messages"][-1]
    uses = [b for b in last.get("content", []) if getattr(b, "type", None) == "tool_use"]
    return "tools" if uses else "check_output"


def after_tools(state: AgentState) -> str:
    return END if state.get("outcome") else "agent"


def build_graph():
    g = StateGraph(AgentState)
    g.add_node("check_input", check_input)
    g.add_node("retrieve", do_retrieve)
    g.add_node("build_context", build_context)
    g.add_node("agent", call_model)
    g.add_node("tools", call_tools)
    g.add_node("check_output", check_output)

    g.add_edge(START, "check_input")
    g.add_conditional_edges("check_input", after_input, {END: END, "retrieve": "retrieve"})
    g.add_edge("retrieve", "build_context")
    g.add_edge("build_context", "agent")
    g.add_conditional_edges("agent", after_model, {END: END, "tools": "tools", "check_output": "check_output"})
    g.add_conditional_edges("tools", after_tools, {END: END, "agent": "agent"})
    g.add_edge("check_output", END)
    return g.compile()


_graph = None


def run(
    question: str,
    principal: Principal,
    resume: dict | None = None,
    approved: set[str] | None = None,
) -> Outcome:
    """Same signature and same return type as the `none` variant.

    The graph is compiled once. `recursion_limit` is set from the manifest's max_steps rather than
    left at the framework default: a bound that lives in two places drifts, and the copy in the
    framework's default is the one nobody reviews.
    """
    global _graph
    if _graph is None:
        _graph = build_graph()

    manifest = load_manifest()
    state: AgentState = {
        "question": question,
        "principal": principal,
        "manifest": manifest,
        "tools": load_tools(manifest),
        "approved": approved or set(),
        "calls": (resume or {}).get("tool_calls", 0),
        "spent": (resume or {}).get("spent_usd", 0.0),
        "passages": [],
    }
    if resume:
        state["messages"] = resume["messages"]

    final = _graph.invoke(state, {"recursion_limit": manifest["autonomy"]["max_steps"] * 3})
    return final.get("outcome") or Outcome(
        status="escalated", text="[ESCALATE] step budget exhausted", cost_usd=final.get("spent", 0.0)
    )


def _serialisable(messages: list) -> list:
    out = []
    for m in messages:
        content = m["content"] if isinstance(m, dict) else getattr(m, "content", "")
        role = m["role"] if isinstance(m, dict) else getattr(m, "type", "user")
        if isinstance(content, str):
            out.append({"role": role, "content": content})
            continue
        out.append(
            {
                "role": role,
                "content": [b if isinstance(b, dict) else (b.model_dump() if hasattr(b, "model_dump") else dict(b)) for b in content],
            }
        )
    return out
