"""Account operations. No model, no HTTP, no agent.

Replace these with calls into your own systems. They live here rather than in agent/tools.py because
"who may close an account" is a rule about your business — and a rule reachable only by asking a
language model cannot be unit tested, reused in an admin screen, or audited without a token budget.

Every function takes an explicit tenant. None of them can answer "all of them": a query with an
optional tenant filter eventually runs without it.
"""

from __future__ import annotations


class NotFound(Exception):
    """The account does not exist, or does not belong to this tenant.

    One exception for both. Distinguishing them tells an unauthorised caller that the account exists,
    which is how an enumeration oracle gets built by accident.
    """


def lookup(account_id: str, tenant_id: str) -> dict:
    """read: safe to repeat, safe to run without asking anybody."""
    return {"account_id": account_id, "tenant_id": tenant_id, "status": "active", "plan": "team"}


def update_contact(account_id: str, email: str, tenant_id: str) -> dict:
    """write: it changes state and can be undone, so at L2 it runs alone."""
    return {"updated": True, "account_id": account_id, "email": email}


def close(account_id: str, tenant_id: str) -> dict:
    """irreversible: it stops for a human however capable the model is.

    The irreversibility is a property of the world, not of the agent, which is why it is asserted
    here and declared in the tool spec rather than being a judgement the model makes at runtime.
    """
    print(f"[would close account {account_id} in tenant {tenant_id}]")
    return {"closed": True, "account_id": account_id}
