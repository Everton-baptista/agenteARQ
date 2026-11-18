"""LangGraph adapter. The graph delegates one turn to the shared execution engine.

Checkpointing across human approval is explicit in Outcome.state, so framework reducers cannot
merge tool arguments or reset budgets. The graph itself never creates a second provider loop.
"""
from typing import TypedDict
from langgraph.graph import StateGraph, START, END
from . import native
from .execution import Outcome, grant_for_state
from ..infra.provider import create_message

class Turn(TypedDict, total=False):
    inputs: dict
    outcome: Outcome

def _execute(state: Turn) -> dict:
    return {"outcome": native.run(**state["inputs"])}

_graph = StateGraph(Turn)
_graph.add_node("bounded_turn", _execute)
_graph.add_edge(START, "bounded_turn")
_graph.add_edge("bounded_turn", END)
_graph = _graph.compile()

def run(question, principal, resume=None, approved=None, **kwargs):
    return _graph.invoke({"inputs": dict(question=question, principal=principal,
        resume=resume, approved=approved, **kwargs)})["outcome"]
