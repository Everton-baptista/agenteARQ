"""The three guardrail points: user input, model output, tool action.

None of them is a prompt instruction. A sentence in the system prompt asking the model to behave
is a preference; these are checks that run in code and can refuse. That distinction is the whole
of standard 08.

The action check must never call the model. A check that shares the model's context shares its
compromise, and this is the one place where that matters most — it is the last thing that runs
before something happens in the world.
"""

from __future__ import annotations

from .principal import Principal

INJECTION_MARKERS = (
    "ignore previous", "ignore the above", "disregard your instructions",
    "you are now", "system prompt", "reveal your instructions",
)

MAX_INPUT_CHARS = 4000


def input_guardrail(text: str) -> tuple[bool, str]:
    """fail_closed.

    Probabilistic and evadable — its job is to raise the cost of the unsophisticated attempt. The
    structural defences are the delimited untrusted block and tool least privilege, and neither
    becomes optional because this function exists. Treating a marker list as the defence is how
    teams end up with one layer they believe in and none that holds.
    """
    if len(text) > MAX_INPUT_CHARS:
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


def action_guardrail(
    name: str, args: dict, spec: dict | None, principal: Principal
) -> tuple[bool, str]:
    """fail_closed. The last checkpoint before something happens.

    Returns whether the call may proceed. Approval is *not* decided here: in a service the
    approver is a person who is not in the request, so this reports that approval is required and
    the transport layer arranges it. Blocking a worker thread on a human decision is how a
    service falls over under the load of its own approvals.
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

    return True, ""


def needs_approval(spec: dict) -> bool:
    """Whether this call may only proceed with a human decision on the record."""
    return spec["effect"] in ("irreversible", "money", "communication")


def render_for_approver(spec: dict, args: dict, principal: Principal) -> dict:
    """What the approver is shown.

    Never just "the agent wants to run notify_customer — approve?". That produces a click, not a
    decision, and a queue of clicks is approval fatigue with an audit trail. The approver gets the
    effect, who it affects, and the actual content.
    """
    return {
        "tool": spec["id"],
        "effect": spec["effect"],
        "reversible": False,
        "approver_role": spec["approval"]["approver_role"],
        "on_timeout": spec["approval"]["on_timeout"],
        "acting_for": {"tenant": principal.tenant_id, "customer": principal.customer_id},
        "arguments": {k: (v if len(str(v)) < 500 else str(v)[:500] + "…") for k, v in args.items()},
    }
