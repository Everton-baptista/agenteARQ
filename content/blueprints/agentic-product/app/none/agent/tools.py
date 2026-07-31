"""Tool implementations and the dispatcher.

Separate from the runner on purpose. The runner is the loop; this is the boundary where a model's
proposed call becomes a real one, and keeping them apart means the guardrail sequence lives in one
readable place instead of being interleaved with turn bookkeeping.

Identity is injected server-side. Every implementation takes the tenant from the principal and never
from the model's arguments — a tool that accepts a tenant id as a parameter has handed the isolation
boundary to whoever can write into the model's context.
"""

from __future__ import annotations

from ..domain import accounts
from .guardrails import action_guardrail, needs_approval
from .principal import Principal


class ApprovalRequired(Exception):
    """Raised when a call may only proceed with a human decision on the record.

    An exception rather than a return value because it has to interrupt the loop. The alternative is
    every caller of dispatch() remembering to check, and one that forgets performs the irreversible
    action silently.
    """

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
    autonomy: str,
    approved: set[str] | None = None,
) -> dict:
    """Run one tool call, or refuse, or pause for a person.

    `approved` is a set of tool names a human has cleared for this run — not a boolean, so approving
    one action does not silently approve the next one in the same conversation.
    """
    ok, reason = action_guardrail(name, args, spec, principal, autonomy)
    if not ok:
        return {"error": reason, "retryable": False}

    assert spec is not None  # action_guardrail refuses when it is None
    if needs_approval(spec) and name not in (approved or set()):
        raise ApprovalRequired(name, args, spec)

    tenant = principal.tenant_id
    if name == "lookup_account":
        return accounts.lookup(args["account_id"], tenant_id=tenant)
    if name == "update_contact":
        return accounts.update_contact(args["account_id"], args["email"], tenant_id=tenant)
    if name == "close_account":
        return accounts.close(args["account_id"], tenant_id=tenant)
    return {"error": "no implementation", "retryable": False}
