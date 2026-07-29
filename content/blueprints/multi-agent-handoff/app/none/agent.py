"""Orchestrator and specialists, with the authority bounded at the boundary.

The first question a multi-agent system should answer is whether it needs to be one. "Specialised
agents" often means one agent with a longer tool list split across three prompts — with three
times the surface and no extra containment.

Where it is genuinely warranted, three things have to hold, and none of them is automatic:
the payload is typed, the authority does not travel with the message, and the budget is shared.

Run it with:  python app/agent.py "I was charged twice for order BR-77120"
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

AGENTS = pathlib.Path("agentarch/project/agents")


def load(agent_id: str) -> dict:
    d = AGENTS / agent_id
    manifest = yaml.safe_load((d / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]
    raw = (d / spec["path"]).read_bytes()
    if hashlib.sha256(raw).hexdigest() != spec["sha256"]:
        raise SystemExit(f"{agent_id}: prompt changed without a version bump")
    manifest["_system_prompt"] = raw.decode()
    manifest["_dir"] = d
    return manifest


@dataclass
class Budget:
    """Shared across the system, not per agent.

    Three agents with ten steps each is a system with thirty, and a delegation cycle between
    them is a system with no bound at all. Per-agent limits do not catch A → B → A.
    """
    max_steps: int
    used: int = 0
    depth: int = 0
    max_depth: int = 3
    path: list[str] = field(default_factory=list)

    def spend(self, agent_id: str) -> tuple[bool, str]:
        self.used += 1
        if self.used > self.max_steps:
            return False, f"shared step budget exhausted after {self.used}"
        if self.depth > self.max_depth:
            return False, f"delegation deeper than {self.max_depth}"
        # The cycle no per-agent limit catches.
        if agent_id in self.path:
            return False, f"delegation cycle: {' → '.join(self.path)} → {agent_id}"
        return True, ""


def validate_payload(payload: dict, schema_path: pathlib.Path) -> tuple[bool, str]:
    """A handoff without a typed payload is not a contract.

    Validated at the boundary, because the sending agent composed it and the receiving agent is
    about to act on it — and one of those two is where an injection would have landed.
    """
    if not schema_path.exists():
        return False, f"payload schema {schema_path.name} is missing"
    schema = json.loads(schema_path.read_text())
    for field_name in schema.get("required", []):
        if field_name not in payload:
            return False, f"payload is missing required field {field_name!r}"
    return True, ""


def hand_off(caller: dict, to_agent: str, payload: dict, budget: Budget) -> dict:
    """Delegate, with the authority the contract declares and no more."""
    contracts = caller.get("handoff", {}).get("hands_off_to", [])
    contract = next((c for c in contracts if c["agent_id"] == to_agent), None)
    if contract is None:
        # Not declared in the manifest, so it does not happen. The alternative is a delegation
        # graph that exists only in the code and that no review ever sees.
        return {"error": f"{to_agent} is not in hands_off_to", "retryable": False}

    ok, reason = validate_payload(payload, caller["_dir"] / contract["payload_schema"])
    if not ok:
        return {"error": reason, "retryable": False}

    ok, reason = budget.spend(to_agent)
    if not ok:
        return {"error": reason, "retryable": False}

    budget.depth += 1
    budget.path.append(to_agent)
    try:
        specialist = load(to_agent)
        # Authority does not travel with the message. The specialist gets the tools its own
        # manifest declares — which is what stops an orchestrator that can issue refunds from
        # making everything it delegates to able to.
        result = run_agent(specialist, json.dumps(payload), budget,
                           authority=contract["authority"])
        return {"return_point": contract["return_point"], "result": result}
    finally:
        budget.depth -= 1
        budget.path.pop()


def run_agent(manifest: dict, task: str, budget: Budget, authority: str = "delegated") -> str:
    client = Anthropic()

    # An incoming task from another agent is untrusted content. It arrives through a channel you
    # built, from a component you wrote, which is exactly why it gets trusted — and exactly why
    # it works as a carrier for something laundered through a document the sender read.
    messages = [{"role": "user", "content": f"<incoming_task>\n{task}\n</incoming_task>"}]

    tools = []
    if authority != "read_only":
        for entry in manifest.get("tools", []):
            spec = yaml.safe_load((manifest["_dir"] / entry["ref"]).read_text())["tool"]
            tools.append({"name": spec["id"], "description": spec["description_for_model"],
                          "input_schema": spec["input_schema"]})

    for _ in range(manifest["autonomy"]["max_steps"]):
        ok, reason = budget.spend(manifest["id"])
        if not ok:
            return f"[ESCALATE] {reason}"

        response = client.messages.create(
            model=manifest["model"]["id"],
            max_tokens=manifest["model"]["params"]["max_output_tokens"],
            system=manifest["_system_prompt"], tools=tools or [], messages=messages)

        uses = [b for b in response.content if b.type == "tool_use"]
        if not uses:
            return "".join(b.text for b in response.content if b.type == "text")

        messages.append({"role": "assistant", "content": response.content})
        messages.append({"role": "user", "content": [
            {"type": "tool_result", "tool_use_id": u.id,
             "content": json.dumps({"error": "not implemented in this blueprint"})}
            for u in uses]})

    return "[ESCALATE] step budget exhausted"


if __name__ == "__main__":
    if not os.getenv("ANTHROPIC_API_KEY"):
        raise SystemExit("set ANTHROPIC_API_KEY")

    orchestrator = load("request-orchestrator")
    # One budget for the whole system.
    budget = Budget(max_steps=orchestrator["autonomy"]["max_steps"],
                    path=["request-orchestrator"])
    print(run_agent(orchestrator, " ".join(sys.argv[1:]) or "I was charged twice", budget))
    print(f"\nsteps used across the system: {budget.used}/{budget.max_steps}")
