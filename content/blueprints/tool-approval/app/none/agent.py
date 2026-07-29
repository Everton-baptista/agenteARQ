"""Task agent at L2, with every irreversible action behind a human.

L2 means the agent acts alone on anything reversible. It does not mean it acts alone on
everything — and the difference is declared in the manifest rather than left to whoever wrote
the dispatch function.

Run it with:  python app/agent.py "close the account for cus-42"
"""

from __future__ import annotations

import hashlib
import json
import os
import pathlib
import sys
from dataclasses import dataclass, field

import yaml
from anthropic import Anthropic

AGENT_DIR = pathlib.Path("agentarch/project/agents/account-ops")


def load_manifest() -> dict:
    manifest = yaml.safe_load((AGENT_DIR / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]
    raw = (AGENT_DIR / spec["path"]).read_bytes()
    if hashlib.sha256(raw).hexdigest() != spec["sha256"]:
        raise SystemExit("system prompt changed without a version bump — run `agentarch validate`")
    manifest["_system_prompt"] = raw.decode()
    return manifest


def load_tools(manifest: dict) -> dict[str, dict]:
    tools = {}
    for entry in manifest.get("tools", []):
        spec = yaml.safe_load((AGENT_DIR / entry["ref"]).read_text())["tool"]
        spec["_approval"] = entry.get("approval", "none")
        tools[spec["id"]] = spec
    return tools


@dataclass
class Session:
    operator_id: str
    role: str
    audit: list = field(default_factory=list)


# ── approval ────────────────────────────────────────────────────────────────────────────────

def request_approval(spec: dict, args: dict, session: Session) -> tuple[bool, str]:
    """Show what will happen, not that something will.

    An approver who cannot see the arguments is approving the agent's judgement rather than the
    action — and the agent's judgement is the thing under question.
    """
    approval = spec["approval"]
    print("\n─── approval required " + "─" * 40)
    print(f"  tool        {spec['id']}")
    print(f"  effect      {spec['effect']}  (cannot be undone)")
    print(f"  requested by {session.operator_id} ({session.role})")
    print(f"  approver    {approval['approver_role']}")
    print("  arguments:")
    for k, v in args.items():
        print(f"    {k:20} {v}")
    print(f"\n  if nobody answers within {approval['timeout_s']}s: {approval['on_timeout']}")

    try:
        answer = input("\n  approve? [y/N] ").strip().lower()
    except (EOFError, KeyboardInterrupt):
        # on_timeout: deny. Silence is not consent, and a system that proceeds when nobody
        # answered has an approval step rather than an approval control.
        session.audit.append({"tool": spec["id"], "args": args, "decision": "timeout_deny"})
        return False, "no answer — denied per on_timeout"

    granted = answer in ("y", "yes")
    session.audit.append({
        "tool": spec["id"], "args": args, "approver": approval["approver_role"],
        "decision": "approved" if granted else "rejected",
        # What the approver was shown is the only thing that makes an approval record answer
        # the question that matters after an incident.
        "shown": list(args.keys()),
    })
    return granted, "" if granted else "rejected by approver"


def action_guardrail(name: str, args: dict, spec: dict | None, session: Session,
                     autonomy: str) -> tuple[bool, str]:
    """fail_closed, and it never calls the model."""
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"

    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        key = limit.replace("max_", "")
        if key in args and isinstance(args[key], (int, float)) and args[key] > cap:
            return False, f"{limit} exceeded: {args[key]} > {cap}"

    irreversible = spec["effect"] in ("irreversible", "money", "communication")
    above_l1 = autonomy not in ("L0_suggest", "L1_act_with_approval")

    if irreversible and above_l1 and spec["_approval"] not in ("human", "policy"):
        # This is the composition the tool schema cannot see: a valid tool, wired up without
        # approval, on an agent that acts alone.
        return False, f"{name} is {spec['effect']} at {autonomy} with approval: none"

    if irreversible:
        return request_approval(spec, args, session)

    return True, ""


# ── tools ───────────────────────────────────────────────────────────────────────────────────

def lookup_account(account_id: str, session: Session) -> dict:
    return {"account_id": account_id, "status": "active", "plan": "team"}


def update_contact(account_id: str, email: str, session: Session) -> dict:
    """write: it changes state and can be undone, so at L2 it runs alone."""
    return {"updated": True, "account_id": account_id, "email": email}


def close_account(account_id: str, session: Session) -> dict:
    """irreversible: it stops for a human however capable the model is."""
    print(f"\n[would close account {account_id}]")
    return {"closed": True, "account_id": account_id}


IMPLS = {"lookup_account": lookup_account, "update_contact": update_contact,
         "close_account": close_account}


def dispatch(name: str, args: dict, tools: dict, session: Session, autonomy: str) -> dict:
    ok, reason = action_guardrail(name, args, tools.get(name), session, autonomy)
    if not ok:
        return {"error": reason, "retryable": False}
    return IMPLS[name](**args, session=session)


def run(instruction: str, session: Session) -> str:
    manifest = load_manifest()
    tools = load_tools(manifest)
    autonomy = manifest["autonomy"]["level"]

    client = Anthropic()
    messages = [{"role": "user", "content":
                 f"<operator_request>\n{instruction}\n</operator_request>"}]
    schemas = [{"name": s["id"], "description": s["description_for_model"],
                "input_schema": s["input_schema"]} for s in tools.values()]

    calls = 0
    for _ in range(manifest["autonomy"]["max_steps"]):
        response = client.messages.create(
            model=manifest["model"]["id"],
            max_tokens=manifest["model"]["params"]["max_output_tokens"],
            system=manifest["_system_prompt"], tools=schemas, messages=messages)

        uses = [b for b in response.content if b.type == "tool_use"]
        if not uses:
            return "".join(b.text for b in response.content if b.type == "text")

        calls += len(uses)
        if calls > manifest["autonomy"]["max_tool_calls"]:
            return "[ESCALATE] tool call budget exhausted"

        messages.append({"role": "assistant", "content": response.content})
        messages.append({"role": "user", "content": [
            {"type": "tool_result", "tool_use_id": u.id,
             "content": json.dumps(dispatch(u.name, u.input, tools, session, autonomy))}
            for u in uses]})

    return "[ESCALATE] step budget exhausted"


if __name__ == "__main__":
    if not os.getenv("ANTHROPIC_API_KEY"):
        raise SystemExit("set ANTHROPIC_API_KEY")
    session = Session(operator_id="op-7", role="support_agent")
    print(run(" ".join(sys.argv[1:]) or "close the account for cus-42", session))
    if session.audit:
        print("\naudit trail:")
        for row in session.audit:
            print(" ", row)
