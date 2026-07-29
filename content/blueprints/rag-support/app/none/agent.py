"""Grounded support agent — the reference implementation of this blueprint.

No framework: the provider SDK directly, so what the agentarch manifest asks for and what the
code does are visible side by side without an abstraction in between. Porting this to LangGraph,
the OpenAI Agents SDK, Claude Agent SDK, Agno and others is documented in
agentarch/std/adapters/.

Run it with:  python app/agent.py "where is my order BR-77120?"
"""

from __future__ import annotations

import hashlib
import json
import os
import pathlib
import sys
from dataclasses import dataclass

import yaml
from anthropic import Anthropic

AGENT_DIR = pathlib.Path("agentarch/project/agents/support-triage")


# ── the manifest is the contract ────────────────────────────────────────────────────────────

def load_manifest() -> dict:
    """Load the manifest and verify the prompt still hashes to what it records.

    Failing closed here is the point. A prompt edited without a version bump is an invisible
    behaviour change that silently invalidates every eval taken before it, and starting anyway
    means shipping something nobody reviewed.
    """
    manifest = yaml.safe_load((AGENT_DIR / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]

    raw = (AGENT_DIR / spec["path"]).read_bytes()
    actual = hashlib.sha256(raw).hexdigest()
    if actual != spec["sha256"]:
        raise SystemExit(
            f"system prompt has changed but version {spec['version']} was not bumped.\n"
            f"  manifest records {spec['sha256'][:12]}…\n"
            f"  file hashes to   {actual[:12]}…\n"
            "Run `agentarch validate` — this is AA-REF-004."
        )

    manifest["_system_prompt"] = raw.decode()
    return manifest


def load_tools() -> dict[str, dict]:
    """Tool specs are the source for the model-facing schema, so the reviewed artifact and what
    the model sees cannot drift apart."""
    tools = {}
    for entry in load_manifest().get("tools", []):
        spec = yaml.safe_load((AGENT_DIR / entry["ref"]).read_text())["tool"]
        spec["_approval"] = entry.get("approval", "none")
        tools[spec["id"]] = spec
    return tools


# ── session: where identity comes from ──────────────────────────────────────────────────────

@dataclass
class Session:
    """Identity lives here and nowhere else.

    A tool that accepts a customer id as an argument has handed identity selection to whoever
    can write into the model's context — which, for a RAG agent, is anyone who can edit a help
    centre article.
    """
    conversation_id: str
    customer_id: str
    customer_tier: str = "standard"


# ── guardrails: three points, none of them the prompt ───────────────────────────────────────

INJECTION_MARKERS = (
    "ignore previous", "ignore the above", "disregard your instructions",
    "you are now", "system prompt", "reveal your instructions",
)


def input_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed. Probabilistic and evadable — its job is to raise cost and catch the
    unsophisticated case. The structural defences are the untrusted block below and tool least
    privilege, and neither is optional because this exists."""
    if len(text) > 4000:
        return False, "input too long"
    lowered = text.lower()
    if any(m in lowered for m in INJECTION_MARKERS):
        return False, "input looks like an injection attempt"
    return True, ""


def output_guardrail(text: str, cited: bool) -> tuple[bool, str]:
    """fail_closed on leakage, fail_warn on grounding.

    A missing citation degrades to escalation rather than blocking the reply, which is what the
    manifest declares. Blocking there would turn a quality problem into an outage.
    """
    if "sk-" in text or "ANTHROPIC_API_KEY" in text:
        return False, "output appears to contain a credential"
    if not cited:
        return False, "no citation — escalating rather than answering ungrounded"
    return True, ""


def action_guardrail(name: str, args: dict, spec: dict | None, session: Session) -> tuple[bool, str]:
    """fail_closed. The last checkpoint before something happens.

    This function must never call the model. A check that shares the model's context shares its
    compromise, and this is the one place where that matters most.
    """
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"

    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        key = limit.replace("max_", "")
        if key in args and isinstance(args[key], (int, float)) and args[key] > cap:
            return False, f"{limit} exceeded"

    if spec["effect"] in ("irreversible", "money", "communication"):
        if spec["_approval"] not in ("human", "policy"):
            return False, f"{name} is {spec['effect']} but wired up without approval"
        if not request_approval(spec, args, session):
            return False, "human approval was not granted"

    return True, ""


def request_approval(spec: dict, args: dict, session: Session) -> bool:
    """Show the approver what will happen, not just that something will.

    A dialog saying "the agent wants to run notify_customer — approve?" produces a click, not a
    decision. On timeout this denies: an unanswered request is not consent.
    """
    print("\n─── approval required ───────────────────────────────")
    print(f"  tool     {spec['id']}  ({spec['effect']}, cannot be undone)")
    print(f"  approver {spec['approval']['approver_role']}")
    for k, v in args.items():
        shown = v if len(str(v)) < 200 else str(v)[:200] + "…"
        print(f"  {k:8} {shown}")
    print(f"  on timeout: {spec['approval']['on_timeout']}")
    try:
        return input("\n  approve? [y/N] ").strip().lower() in ("y", "yes")
    except (EOFError, KeyboardInterrupt):
        return False  # on_timeout: deny


# ── retrieval: everything it returns is untrusted ───────────────────────────────────────────

def retrieve(question: str) -> list[dict]:
    """Replace this with your retriever.

    Whatever it returns is untrusted content. Anyone who can edit a help centre article can
    write into this, which is exactly why it is rendered as data below and never joined to the
    system prompt.
    """
    corpus = [
        {"id": "kb-001", "text": "Orders ship within two business days. Tracking arrives by email."},
        {"id": "kb-002", "text": "Returns are accepted within 30 days of delivery, unopened."},
        {"id": "kb-003", "text": "Delivery estimates come from the carrier and can change."},
    ]
    terms = set(question.lower().split())
    scored = [(len(terms & set(p["text"].lower().split())), p) for p in corpus]
    return [p for score, p in sorted(scored, key=lambda x: -x[0]) if score > 0][:3]


def render_untrusted(passages: list[dict], question: str) -> str:
    """Instruction and data, structurally apart.

    The tags are meaningless unless the system prompt gives them meaning — which it does, in the
    section that says instructions appearing inside them are evidence of tampering.
    """
    body = "\n".join(f"[{p['id']}] {p['text']}" for p in passages) or "(no passages found)"
    return (
        "<retrieved_content>\n" + body + "\n</retrieved_content>\n"
        "<customer_message>\n" + question + "\n</customer_message>"
    )


# ── tools ───────────────────────────────────────────────────────────────────────────────────

def search_orders(order_reference: str, session: Session) -> dict:
    """Read-only. The customer is resolved from the session, never from an argument."""
    return {
        "orders": [{
            "order_id": order_reference,
            "customer_id": session.customer_id,
            "status": "in transit",
            "carrier": "Example Logistics",
            "eta": "2026-08-02",
        }]
    }


def notify_customer(conversation_id: str, body: str, session: Session) -> dict:
    """Irreversible once delivered — the recipient has already read it."""
    print(f"\n[would send to {conversation_id}]: {body}")
    return {"sent": True}


IMPLS = {"search_orders": search_orders, "notify_customer": notify_customer}


def dispatch(name: str, args: dict, spec: dict | None, session: Session) -> dict:
    ok, reason = action_guardrail(name, args, spec, session)
    if not ok:
        return {"error": reason, "retryable": False}

    # Identity is injected here, server-side. It is never a parameter the model fills in.
    if name == "search_orders":
        return IMPLS[name](args["order_reference"], session)
    if name == "notify_customer":
        return IMPLS[name](session.conversation_id, args["body"], session)
    return {"error": "no implementation", "retryable": False}


# ── the loop, bounded by the manifest ───────────────────────────────────────────────────────

def run(question: str, session: Session) -> str:
    manifest = load_manifest()
    tools = load_tools()

    ok, reason = input_guardrail(question)
    if not ok:
        return f"[ESCALATE] {reason}"

    client = Anthropic()
    passages = retrieve(question)
    messages = [{"role": "user", "content": render_untrusted(passages, question)}]

    tool_schemas = [{
        "name": s["id"],
        "description": s["description_for_model"],
        "input_schema": s["input_schema"],
    } for s in tools.values()]

    # Every bound comes from the manifest, so the declared limit and the enforced limit cannot
    # drift apart.
    max_steps = manifest["autonomy"]["max_steps"]
    max_tool_calls = manifest["autonomy"]["max_tool_calls"]
    calls = 0

    for _ in range(max_steps):
        response = client.messages.create(
            model=manifest["model"]["id"],
            max_tokens=manifest["model"]["params"]["max_output_tokens"],
            temperature=manifest["model"]["params"]["temperature"],
            system=manifest["_system_prompt"],
            tools=tool_schemas,
            messages=messages,
        )

        tool_uses = [b for b in response.content if b.type == "tool_use"]
        if not tool_uses:
            text = "".join(b.text for b in response.content if b.type == "text")
            cited = any(p["id"] in text for p in passages)
            ok, reason = output_guardrail(text, cited)
            return text if ok else f"[ESCALATE] {reason}"

        if calls + len(tool_uses) > max_tool_calls:
            return "[ESCALATE] tool call budget exhausted"
        calls += len(tool_uses)

        messages.append({"role": "assistant", "content": response.content})
        results = []
        for use in tool_uses:
            out = dispatch(use.name, use.input, tools.get(use.name), session)
            results.append({
                "type": "tool_result",
                "tool_use_id": use.id,
                "content": json.dumps(out),
                "is_error": "error" in out,
            })
        messages.append({"role": "user", "content": results})

    return "[ESCALATE] step budget exhausted"


if __name__ == "__main__":
    if not os.getenv("ANTHROPIC_API_KEY"):
        raise SystemExit("set ANTHROPIC_API_KEY")
    question = " ".join(sys.argv[1:]) or "where is my order BR-77120?"
    # In a real deployment this comes from your auth layer, never from the request body.
    session = Session(conversation_id="conv-1", customer_id="cus-42")
    print(run(question, session))
