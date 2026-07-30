"""Tool implementations and the dispatcher — separate from the runner, and from handoff.

Three modules where a single loop would do, and the split is the design:

  runner.py    one agent's turn
  handoff.py   the boundary between two agents
  tools.py     the boundary between a model's proposal and a real action

Interleaving them is how a multi-agent system ends up with the authority check in one place, the
budget check in another, and a path through the code that misses one. Each boundary is one file, and
each has tests that need no model.
"""

from __future__ import annotations

from ..domain import orders
from .guardrails import action_guardrail
from .principal import Principal


class ApprovalRequired(Exception):
    """A call that may only proceed with a human decision on the record."""

    def __init__(self, tool: str, args: dict, spec: dict):
        super().__init__(f"{tool} requires approval")
        self.tool = tool
        self.args = args
        self.spec = spec


def dispatch(
    name: str,
    args: dict,
    spec: dict | None,
    principal: Principal,
    authority: str,
    approved: set[str] | None = None,
) -> dict:
    """Run one tool call, or refuse.

    `authority` is what the handoff contract granted for this run, and it can only ever narrow what
    the agent's own manifest allows. A delegated agent that could widen its authority would make the
    contract advisory.
    """
    ok, reason = action_guardrail(name, args, spec, principal, authority)
    if not ok:
        return {"error": reason, "retryable": False}

    assert spec is not None
    if spec["effect"] in ("irreversible", "money", "communication") and name not in (approved or set()):
        raise ApprovalRequired(name, args, spec)

    tenant = principal.tenant_id
    if name == "search_orders":
        return orders.lookup(args["order_reference"], tenant_id=tenant)
    if name == "notify_customer":
        # conversation_id is server-side state, never a model argument.
        return orders.send_message(args.get("_conversation_id", "conv-1"), args["body"], tenant_id=tenant)
    return {"error": "no implementation", "retryable": False}
