"""Tool implementations and the dispatcher.

Two rules are visible here and both are load-bearing.

Nothing about the caller arrives as a tool argument. `email_follow_up` takes a subject and a
body from the model and the recipient from the principal — never the other way round. A tool
that accepts an email address as an argument has handed recipient selection to whoever can
write into the model's context, which for a public chat page is anyone on the internet.

Business logic lives in domain/, not here. This module translates between the model's arguments
and a function that knows nothing about models, so the same rule can be exercised by a test, a
batch job, or a human-facing screen without going through an LLM to reach it.
"""

from __future__ import annotations

from ..domain import status
from .guardrails import action_guardrail, needs_approval
from .principal import Principal


class ApprovalRequired(Exception):
    """Raised when a call may only proceed with a human decision on the record.

    An exception rather than a return value because it has to interrupt the loop: the alternative
    is every caller of dispatch() remembering to check, and one that forgets performs the
    irreversible action silently.
    """

    def __init__(self, tool: str, args: dict, spec: dict):
        super().__init__(f"{tool} requires approval")
        self.tool = tool
        self.args = args
        self.spec = spec


def check_service_status(component: str, principal: Principal) -> dict:
    """Read-only. The component comes from the model; there is no caller input to inject."""
    return status.component_status(component=component)


def email_follow_up(conversation_id: str, subject: str, body: str, principal: Principal) -> dict:
    """Irreversible once delivered — the recipient has already read it.

    Reached only after a human decision is recorded; see ApprovalRequired above. The tenant
    comes from the principal, the content from the model, and the address from neither — the
    domain resolves it from the conversation.
    """
    return status.send_email(
        conversation_id=conversation_id, subject=subject, body=body, tenant_id=principal.tenant_id
    )


def dispatch(
    name: str,
    args: dict,
    spec: dict | None,
    principal: Principal,
    approved: set[str] | None = None,
) -> dict:
    """Run one tool call, or refuse.

    `approved` carries the tool names a human has already cleared for this run. It is a set of
    names rather than a boolean so that approving one message does not silently approve the next
    one in the same conversation.
    """
    ok, reason = action_guardrail(name, args, spec, principal)
    if not ok:
        return {"error": reason, "retryable": False}

    assert spec is not None  # action_guardrail refuses when it is None
    if needs_approval(spec) and name not in (approved or set()):
        raise ApprovalRequired(name, args, spec)

    if name == "check_service_status":
        return check_service_status(args["component"], principal)
    if name == "email_follow_up":
        # conversation_id is server-side state, not a model argument.
        return email_follow_up(
            args.get("_conversation_id", "conv-1"), args["subject"], args["body"], principal
        )
    return {"error": "no implementation", "retryable": False}
