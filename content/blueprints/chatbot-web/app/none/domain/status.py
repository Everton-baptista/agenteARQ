"""Business logic. No model, no HTTP, no agent.

Replace these with calls into your own systems. The reason they live here rather than inside
agent/tools.py is that a rule about who may be emailed is a rule about your business, not about
the agent — and a rule reachable only by asking a language model is one you cannot unit test,
cannot reuse in a support screen, and cannot audit without a token budget.

Note what every function takes: an explicit `component` or `tenant_id`. None of them has a way
to answer "all of them". A query with an optional tenant filter eventually runs without it.
"""

from __future__ import annotations

# The closed set the tool schema declares. The model can pick one of these and nothing else,
# which is what makes a read tool safe to leave unapproved: there is no identifier to guess.
COMPONENTS = ("api", "dashboard", "webhooks")


class NotFound(Exception):
    """The component does not exist.

    Raised rather than returning a made-up status: a status page that hallucinates "operational"
    is worse than one that admits it does not know.
    """


def component_status(component: str) -> dict:
    """Read the status of one component.

    In your implementation this calls your status page or monitoring API. Keep the closed set:
    the day this accepts a free-form string it accepts whatever an injected instruction types.
    """
    if component not in COMPONENTS:
        raise NotFound(component)
    return {
        "component": component,
        "state": "operational",
        "verified_at": "2026-07-29T00:00:00Z",
    }


def send_email(conversation_id: str, subject: str, body: str, tenant_id: str) -> dict:
    """Deliver a follow-up email to the visitor. Irreversible once it lands.

    In your implementation this is where the outbound provider is called. Keep it here rather
    than in the tool: the fact that an email cannot be unsent is a property of the world, and
    the domain is where properties of the world belong.

    The recipient is resolved from the conversation, server-side. It is deliberately not a
    parameter: an address the model can choose is an address an injection can choose.
    """
    print(f"[would email conversation {conversation_id} in tenant {tenant_id}]: {subject}\n{body}")
    return {"sent": True, "conversation_id": conversation_id}
