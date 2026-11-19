"""Native provider adapter over the deterministic execution engine."""
from ..infra.provider import create_message
from .manifest import load_manifest, load_tools
from .guardrails import input_guardrail, output_guardrail, render_for_approver
from .tools import dispatch
from .execution import Outcome, run_loop, grant_for_state
from functools import lru_cache
from ..infra.store import default_store
from .memory import Memory, render_for_prompt

@lru_cache(maxsize=8)
def _memory(ttl):
    return Memory(default_store(), retention_days=ttl)


def run(question, principal, resume=None, approved=None, memory=None, conversation_id="default"):
    manifest = load_manifest()
    tools = load_tools(manifest)
    passages = []
    content = f"<operator_request>\n{question}\n</operator_request>"
    if memory is None:
        memory = _memory(manifest["context"]["memory"]["ttl_days"])
    if not resume:
        content = render_for_prompt(memory.recall(principal, conversation_id)) + "\n" + content
    return run_loop(question, principal, manifest=manifest, tools=tools, create_message=create_message,
        dispatch=lambda name, args, spec, p, approved=None, call_id="": dispatch(name, args, spec, p, manifest["autonomy"]["level"], approved=approved, call_id=call_id),
        input_guardrail=input_guardrail, output_guardrail=lambda text, cited: output_guardrail(text),
        render_for_approver=render_for_approver, initial_content=content, passages=passages,
        resume=resume, approved=approved, memory=memory, conversation_id=conversation_id)
