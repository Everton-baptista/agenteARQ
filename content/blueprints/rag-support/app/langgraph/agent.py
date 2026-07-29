"""Grounded support agent, on LangGraph.

The same manifest, the same tool specs, the same prompt — a different place to attach them.
Compare with the framework-free version in agentarch/std/adapters/none-raw-sdk.md to see what the
graph changes and what it does not: none of the guarantees move.

State is where agentarch attaches. It is the one thing every node sees, so it carries the
verified prompt, the session identity and the guardrail verdicts.

Run it with:  python app/agent.py "where is my order BR-77120?"
"""

from __future__ import annotations

import hashlib
import json
import os
import pathlib
import sys
from dataclasses import dataclass
from typing import Annotated, TypedDict

import yaml
from langchain_anthropic import ChatAnthropic
from langchain_core.messages import AIMessage, HumanMessage, SystemMessage, ToolMessage
from langchain_core.tools import tool
from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import add_messages

AGENT_DIR = pathlib.Path("agentarch/project/agents/support-triage")


@dataclass
class Session:
    """Identity lives here and nowhere else.

    A tool taking a customer id as an argument has handed identity selection to whoever can write
    into the model's context — for a RAG agent, anyone who can edit a help centre article.
    """
    conversation_id: str
    customer_id: str


class AgentState(TypedDict):
    manifest: dict
    system_prompt: str
    session: Session
    passages: list
    steps: int
    messages: Annotated[list, add_messages]


def load_verified() -> tuple[dict, str]:
    """Fail closed on a prompt that changed without a version bump.

    Starting anyway means shipping something nobody reviewed, and it silently invalidates every
    eval taken before the edit.
    """
    manifest = yaml.safe_load((AGENT_DIR / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]
    raw = (AGENT_DIR / spec["path"]).read_bytes()
    if hashlib.sha256(raw).hexdigest() != spec["sha256"]:
        raise SystemExit(
            f"system prompt changed but {spec['version']} was not bumped — "
            "run `agentarch validate` (AA-REF-004)"
        )
    return manifest, raw.decode()


def load_tool_specs(manifest: dict) -> dict[str, dict]:
    out = {}
    for entry in manifest.get("tools", []):
        spec = yaml.safe_load((AGENT_DIR / entry["ref"]).read_text())["tool"]
        spec["_approval"] = entry.get("approval", "none")
        out[spec["id"]] = spec
    return out


# ── guardrails ──────────────────────────────────────────────────────────────────────────────

INJECTION_MARKERS = ("ignore previous", "disregard your instructions", "you are now",
                     "reveal your instructions")


def input_guardrail(state: AgentState) -> AgentState:
    """A node before the agent node, with a conditional edge to escalate. fail_closed."""
    text = state["messages"][-1].content
    if len(text) > 4000 or any(m in text.lower() for m in INJECTION_MARKERS):
        return {"messages": [AIMessage(content="[ESCALATE] input rejected")]}
    return {}


def retrieve(state: AgentState) -> AgentState:
    """Whatever this returns is untrusted content, and stays in its own state key.

    It is rendered into a delimited block by the node that builds the message — never appended
    to system_prompt.
    """
    corpus = [
        {"id": "kb-001", "text": "Orders ship within two business days."},
        {"id": "kb-002", "text": "Returns are accepted within 30 days of delivery."},
    ]
    q = set(state["messages"][-1].content.lower().split())
    hits = [p for p in corpus if q & set(p["text"].lower().split())]
    return {"passages": hits[:3]}


def build_context(state: AgentState) -> AgentState:
    body = "\n".join(f"[{p['id']}] {p['text']}" for p in state["passages"]) or "(none)"
    question = state["messages"][-1].content
    return {"messages": [HumanMessage(content=(
        "<retrieved_content>\n" + body + "\n</retrieved_content>\n"
        "<customer_message>\n" + question + "\n</customer_message>"
    ))]}


def action_guardrail(name: str, args: dict, spec: dict | None, session: Session) -> tuple[bool, str]:
    """fail_closed, and it never calls the model.

    A check that shares the model's context shares its compromise, and this is the last
    checkpoint before something happens.
    """
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"
    if spec["effect"] in ("irreversible", "money", "communication"):
        if spec["_approval"] not in ("human", "policy"):
            return False, f"{name} is {spec['effect']} but wired up without approval"
        return False, "human approval required — use interrupt() in a real deployment"
    return True, ""


# ── tools ───────────────────────────────────────────────────────────────────────────────────

@tool
def search_orders(order_reference: str) -> dict:
    """Look up an order belonging to the authenticated customer."""
    return {"order_id": order_reference, "status": "in transit", "eta": "2026-08-02"}


TOOL_FNS = {"search_orders": search_orders}


def call_model(state: AgentState) -> AgentState:
    manifest = state["manifest"]
    llm = ChatAnthropic(
        model=manifest["model"]["id"],
        temperature=manifest["model"]["params"]["temperature"],
        max_tokens=manifest["model"]["params"]["max_output_tokens"],
    ).bind_tools(list(TOOL_FNS.values()))

    messages = [SystemMessage(content=state["system_prompt"]), *state["messages"]]
    return {"messages": [llm.invoke(messages)], "steps": state["steps"] + 1}


def call_tools(state: AgentState) -> AgentState:
    specs = load_tool_specs(state["manifest"])
    session = state["session"]
    out = []

    for call in state["messages"][-1].tool_calls:
        ok, reason = action_guardrail(call["name"], call["args"], specs.get(call["name"]), session)
        if not ok:
            out.append(ToolMessage(content=json.dumps({"error": reason}), tool_call_id=call["id"]))
            continue
        # Identity is injected here, server-side, never trusted from the model.
        result = TOOL_FNS[call["name"]].invoke(call["args"])
        out.append(ToolMessage(content=json.dumps(result), tool_call_id=call["id"]))

    return {"messages": out}


def should_continue(state: AgentState) -> str:
    last = state["messages"][-1]
    # The step bound comes from the manifest, so the declared limit and the enforced limit
    # cannot drift apart.
    if state["steps"] >= state["manifest"]["autonomy"]["max_steps"]:
        return "escalate"
    if getattr(last, "tool_calls", None):
        return "tools"
    return "output"


def output_guardrail(state: AgentState) -> AgentState:
    """fail_warn on grounding: a missing citation degrades to escalation rather than blocking."""
    text = state["messages"][-1].content
    cited = any(p["id"] in text for p in state["passages"])
    if not cited:
        return {"messages": [AIMessage(content="[ESCALATE] no citation — not answering ungrounded")]}
    return {}


def escalate(state: AgentState) -> AgentState:
    return {"messages": [AIMessage(content="[ESCALATE] step budget exhausted")]}


def build_graph():
    g = StateGraph(AgentState)
    g.add_node("input_guardrail", input_guardrail)
    g.add_node("retrieve", retrieve)
    g.add_node("build_context", build_context)
    g.add_node("agent", call_model)
    g.add_node("tools", call_tools)
    g.add_node("output_guardrail", output_guardrail)
    g.add_node("escalate", escalate)

    g.add_edge(START, "input_guardrail")
    g.add_edge("input_guardrail", "retrieve")
    g.add_edge("retrieve", "build_context")
    g.add_edge("build_context", "agent")
    g.add_conditional_edges("agent", should_continue, {
        "tools": "tools", "output": "output_guardrail", "escalate": "escalate",
    })
    g.add_edge("tools", "agent")
    g.add_edge("output_guardrail", END)
    g.add_edge("escalate", END)
    return g.compile()


if __name__ == "__main__":
    if not os.getenv("ANTHROPIC_API_KEY"):
        raise SystemExit("set ANTHROPIC_API_KEY")

    manifest, prompt = load_verified()
    question = " ".join(sys.argv[1:]) or "where is my order BR-77120?"

    final = build_graph().invoke({
        "manifest": manifest,
        "system_prompt": prompt,
        # In a real deployment this comes from your auth layer, never from the request body.
        "session": Session(conversation_id="conv-1", customer_id="cus-42"),
        "passages": [],
        "steps": 0,
        "messages": [HumanMessage(content=question)],
    })
    print(final["messages"][-1].content)
