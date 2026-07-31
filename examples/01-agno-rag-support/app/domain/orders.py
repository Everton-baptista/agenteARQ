"""Business logic. No model, no HTTP, no agent.

Replace these with calls into your own systems. The reason they live here rather than inside
agent/tools.py is that a rule about who may see an order is a rule about your business, not about
the agent — and a rule reachable only by asking a language model is one you cannot unit test,
cannot reuse in a support screen, and cannot audit without a token budget.

Note what every function takes: an explicit `customer_id` or `tenant_id`. None of them has a way
to answer "all of them". A query with an optional tenant filter eventually runs without it.
"""

from __future__ import annotations


class NotFound(Exception):
    """The record does not exist, or does not belong to this caller.

    Deliberately one exception for both. Distinguishing them tells an unauthorised caller that the
    record exists, which is how an enumeration oracle gets built by accident.
    """


def lookup(order_reference: str, customer_id: str) -> dict:
    """Read one order, scoped to the customer who is asking.

    The scoping is a parameter with no default. That is the whole defence: an injected instruction
    can propose any order reference it likes and still cannot reach another customer's record.
    """
    return {
        "orders": [
            {
                "order_id": order_reference,
                "customer_id": customer_id,
                "status": "in transit",
                "carrier": "Example Logistics",
                "eta": "2026-08-02",
            }
        ]
    }


def send_message(conversation_id: str, body: str, tenant_id: str) -> dict:
    """Deliver a message to the customer. Irreversible once it lands.

    In your implementation this is where the outbound provider is called. Keep it here rather than
    in the tool: the fact that a message cannot be unsent is a property of the world, and the
    domain is where properties of the world belong.
    """
    print(f"[would send to {conversation_id} in tenant {tenant_id}]: {body}")
    return {"sent": True, "conversation_id": conversation_id}
