"""Business logic. No model, no HTTP, no agent.

Every function takes an explicit tenant, and none of them can answer "all of them" — a query with an
optional tenant filter eventually runs without it. That matters more in a multi-agent system, because
a specialist reached by delegation is one more place the boundary could be dropped.
"""

from __future__ import annotations


class NotFound(Exception):
    """Does not exist, or does not belong to this tenant. One exception for both: distinguishing them
    tells an unauthorised caller that the record exists."""


def lookup(order_reference: str, tenant_id: str) -> dict:
    """read: safe to repeat, and the only effect a read_only delegation may reach."""
    return {
        "order_id": order_reference,
        "tenant_id": tenant_id,
        "status": "in transit",
        "carrier": "Example Logistics",
        "eta": "2026-08-02",
    }


def send_message(conversation_id: str, body: str, tenant_id: str) -> dict:
    """communication: irreversible once delivered — the recipient has already read it.

    Reachable by the orchestrator, and refused for an agent delegated `read_only` authority. That
    refusal is the point of the blueprint: authority does not travel with the message.
    """
    print(f"[would send to {conversation_id} in tenant {tenant_id}]: {body}")
    return {"sent": True, "conversation_id": conversation_id}
