"""Handoff: the typed contract between two agents, and the dispatcher that honours it.

Separated from the runner deliberately. The runner is one agent's loop; this is the boundary between
two of them — and that boundary is where the four things that make multi-agent systems fail are
either checked or not:

  declared      the target must appear in the caller's `hands_off_to`. A delegation graph that exists
                only in code is one no review ever sees.
  typed         the payload is validated against the schema the contract names. A handoff without a
                typed payload is not a contract, it is a hope.
  bounded       the shared budget decides, including the cycle check that per-agent limits cannot
                make.
  no authority  the specialist gets the tools its own manifest declares. Authority does not travel
                with the message — otherwise an orchestrator that can issue refunds makes everything
                it delegates to able to issue refunds.

Keeping this out of the runner means the termination properties can be tested without a model, which
is the difference between a claim about termination and a test for it.
"""

from __future__ import annotations

import json
import pathlib
from typing import Callable

from ..infra import telemetry
from .budget import Budget
from .manifest import load_manifest
from .principal import Principal


class HandoffRefused(Exception):
    """The handoff did not happen, and why. Never retried: every reason here is permanent."""


def validate_payload(payload: dict, schema_path: pathlib.Path) -> tuple[bool, str]:
    """Check the payload against the schema the contract names.

    Validated at the boundary, because the sending agent composed the payload and the receiving agent
    is about to act on it — and one of those two is where an injection would have landed. Validating
    inside the receiver instead would mean the sender's own output was never checked.

    Deliberately a small subset of JSON Schema: required fields, types, and enums. A full validator is
    a dependency, and the fields that matter in a handoff contract are few enough to check honestly.
    """
    if not schema_path.exists():
        return False, f"payload schema {schema_path.name} is missing"

    schema = json.loads(schema_path.read_text())

    for name in schema.get("required", []):
        if name not in payload:
            return False, f"payload is missing required field {name!r}"

    types = {"string": str, "integer": int, "number": (int, float), "boolean": bool,
             "array": list, "object": dict}
    for name, spec in (schema.get("properties") or {}).items():
        if name not in payload:
            continue
        expected = types.get(spec.get("type"))
        if expected and not isinstance(payload[name], expected):
            return False, f"payload field {name!r} should be {spec['type']}"
        if "enum" in spec and payload[name] not in spec["enum"]:
            return False, f"payload field {name!r} is not one of {spec['enum']}"

    if schema.get("additionalProperties") is False:
        extra = set(payload) - set(schema.get("properties") or {})
        if extra:
            # Extra fields are refused rather than dropped. A field the receiver ignores is a field
            # the sender believes was delivered.
            return False, f"payload has undeclared field(s): {', '.join(sorted(extra))}"

    return True, ""


def hand_off(
    caller: dict,
    to_agent: str,
    payload: dict,
    budget: Budget,
    principal: Principal,
    run_agent: Callable[..., str],
) -> dict:
    """Delegate, with the authority the contract declares and no more.

    `run_agent` is passed in rather than imported, which keeps this module free of a circular import
    and — more usefully — makes every property above testable with a stub in place of a model.
    """
    contracts = caller.get("handoff", {}).get("hands_off_to", [])
    contract = next((c for c in contracts if c["agent_id"] == to_agent), None)
    if contract is None:
        return {"error": f"{to_agent} is not in hands_off_to", "retryable": False}

    ok, reason = validate_payload(payload, caller["_dir"] / contract["payload_schema"])
    if not ok:
        return {"error": reason, "retryable": False}

    ok, reason = budget.may_delegate_to(to_agent)
    if not ok:
        return {"error": reason, "retryable": False}

    with telemetry.span(
        "agent.handoff",
        **{
            "agent.from": caller["id"],
            "agent.to": to_agent,
            "agent.authority": contract["authority"],
            "agent.depth": budget.depth + 1,
            "tenant": principal.tenant_id,
        },
    ):
        budget.enter(to_agent)
        try:
            specialist = load_manifest(to_agent)
            result = run_agent(
                specialist,
                json.dumps(payload),
                budget,
                principal,
                authority=contract["authority"],
            )
        finally:
            # In a finally block because a specialist that raises must still pop the path. Without
            # this, one failure makes every later delegation to that agent look like a cycle.
            budget.leave()

    return {
        "return_point": contract["return_point"],
        "result": result,
        "budget": budget.snapshot(),
    }
