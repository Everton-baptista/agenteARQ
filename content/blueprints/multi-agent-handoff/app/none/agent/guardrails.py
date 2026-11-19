"""The three guardrail points, plus the one this blueprint adds: the handoff boundary.

The action check never calls the model — a check that shares the model's context shares its
compromise. In a multi-agent system that matters more, not less: a compromised orchestrator would
otherwise be asking its own attacker whether the delegation is allowed.
"""

from __future__ import annotations

import re

from .principal import Principal
from .authorization import validate_arguments

# Any provider's credential, not one vendor's: a key name such as OPENAI_API_KEY or
# AWS_BEARER_TOKEN_BEDROCK_API_KEY, or an "sk-" style secret.
_CREDENTIAL_RE = re.compile(r"sk-|\b[A-Z][A-Z0-9_]*(?:API_KEY|_TOKEN|SECRET_ACCESS_KEY)\b")

INJECTION_MARKERS = (
    "ignore previous", "ignore the above", "disregard your instructions",
    "you are now", "system prompt", "reveal your instructions",
)

MAX_INPUT_CHARS = 4000


def input_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed. Runs once, at the edge — not per agent.

    A specialist receives a payload composed by another agent, and that payload is checked by the
    handoff's schema validation rather than by this. Running a prompt-injection heuristic over a
    machine-generated JSON payload produces false positives and no security.
    """
    if len(text) > MAX_INPUT_CHARS:
        return False, "input too long"
    lowered = text.lower()
    if any(m in lowered for m in INJECTION_MARKERS):
        return False, "input looks like an injection attempt"
    return True, ""


def output_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed on leakage. Applied to what leaves the system, not to what one agent tells another.

    An intermediate result travels inside a typed payload; the schema is its check. This one runs on
    the reply that reaches the caller.
    """
    if _CREDENTIAL_RE.search(text) is not None:
        return False, "output appears to contain a credential"
    return True, ""


def action_guardrail(
    name: str, args: dict, spec: dict | None, principal: Principal, authority: str
) -> tuple[bool, str]:
    """fail_closed. Never consults the model.

    `authority` is what the handoff contract granted this agent for this run. It is checked here in
    addition to the tool's own effect, because delegated authority must be able to be narrower than
    the specialist's own manifest — an agent invoked with `authority: read_only` may not write even if
    its manifest allows it.
    """
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"

    if spec.get("id") != name:
        return False, "tool name does not match its reviewed contract"
    valid, reason = validate_arguments(args, spec)
    if not valid:
        return False, reason

    effect = spec["effect"]

    if authority == "read_only" and effect != "read":
        return False, f"{name} is {effect} but this agent was delegated read_only authority"

    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        key = limit.replace("max_", "")
        if key in args and isinstance(args[key], (int, float)) and args[key] > cap:
            return False, f"{limit} exceeded: {args[key]} > {cap}"

    if effect in ("irreversible", "money", "communication") and spec["_approval"] not in ("human", "policy"):
        return False, f"{name} is {effect} but wired up without approval"

    return True, ""
