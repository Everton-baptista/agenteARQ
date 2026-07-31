"""The three guardrail points, with the one this blueprint is about: the action check.

The action check is the last thing that runs before something happens in the world, and it must
never call the model. A check that shares the model's context shares its compromise.

What it enforces here, in order, and the order is deliberate:

  1. is this tool declared for this agent at all
  2. may this operator's role even ask for an effect like this — before an approval is raised, so a
     support agent cannot fill an approver's queue with requests to close accounts. Approval fatigue
     is manufactured exactly that way
  3. are the domain limits respected
  4. does the composition of effect, autonomy and approval hold — the check the tool schema cannot
     make on its own, because a perfectly valid tool wired onto an agent that acts alone is the
     failure

Approval itself is not decided here. In a service the approver is not in the request, so this
reports that a decision is required and the transport arranges it.
"""

from __future__ import annotations

from .principal import Principal

INJECTION_MARKERS = (
    "ignore previous", "ignore the above", "disregard your instructions",
    "you are now", "system prompt", "reveal your instructions",
)

MAX_INPUT_CHARS = 4000

IRREVERSIBLE_EFFECTS = ("irreversible", "money", "communication")


def input_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed.

    Probabilistic and evadable; its job is to raise the cost of the unsophisticated attempt. The
    structural defences are the delimited request block and tool least privilege, and neither
    becomes optional because this exists.
    """
    if len(text) > MAX_INPUT_CHARS:
        return False, "input too long"
    lowered = text.lower()
    if any(m in lowered for m in INJECTION_MARKERS):
        return False, "input looks like an injection attempt"
    return True, ""


def output_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed on leakage.

    No grounding requirement here: this agent acts rather than answers from documents, so a citation
    rule would be a check with nothing to check.
    """
    if "sk-" in text or "ANTHROPIC_API_KEY" in text:
        return False, "output appears to contain a credential"
    return True, ""


def action_guardrail(
    name: str, args: dict, spec: dict | None, principal: Principal, autonomy: str
) -> tuple[bool, str]:
    """fail_closed. Never consults the model."""
    if spec is None:
        return False, f"tool {name!r} is not declared for this agent"

    effect = spec["effect"]

    if not principal.may_request(effect):
        return False, f"role {principal.role!r} may not request a {effect} action"

    for limit, cap in spec.get("limits", {}).get("domain_limits", {}).items():
        key = limit.replace("max_", "")
        if key in args and isinstance(args[key], (int, float)) and args[key] > cap:
            return False, f"{limit} exceeded: {args[key]} > {cap}"

    irreversible = effect in IRREVERSIBLE_EFFECTS
    above_l1 = autonomy not in ("L0_suggest", "L1_act_with_approval")

    if irreversible and above_l1 and spec["_approval"] not in ("human", "policy"):
        # The composition the tool schema cannot see: a valid tool, wired up without approval, on an
        # agent that acts alone. This is control.ai.tool.irreversible_requires_approval, in code.
        return False, f"{name} is {effect} at {autonomy} with approval: none"

    return True, ""


def needs_approval(spec: dict) -> bool:
    return spec["effect"] in IRREVERSIBLE_EFFECTS


def render_for_approver(spec: dict, args: dict, principal: Principal) -> dict:
    """What the approver is shown.

    Never just "the agent wants to run close_account — approve?". An approver who cannot see the
    arguments is approving the agent's judgement, and the agent's judgement is the thing in question.
    """
    approval = spec.get("approval", {})
    return {
        "tool": spec["id"],
        "effect": spec["effect"],
        "reversible": False,
        "requested_by": {"subject": principal.subject, "role": principal.role},
        "approver_role": approval.get("approver_role", "unknown"),
        "timeout_s": approval.get("timeout_s"),
        "on_timeout": approval.get("on_timeout", "deny"),
        "arguments": {k: (v if len(str(v)) < 500 else str(v)[:500] + "…") for k, v in args.items()},
    }
