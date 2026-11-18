"""Development account adapter with real local state; never enabled in production."""
from __future__ import annotations

import copy
import os
import threading

_lock = threading.RLock()
_accounts = {(tenant, "acct-4471"): {"account_id": "acct-4471", "status": "active", "plan": "team", "email": "contact@example.invalid"}
             for tenant in ("acme", "local", "eval")}


class NotFound(Exception):
    pass


def _record(account_id: str, tenant_id: str) -> dict | None:
    if os.getenv("ENV") == "production":
        raise RuntimeError("the development account adapter is disabled in production")
    return _accounts.get((tenant_id, account_id))


def lookup(account_id: str, tenant_id: str) -> dict:
    with _lock:
        record = _record(account_id, tenant_id)
        return copy.deepcopy(record) if record else {"error": "account not found", "retryable": False}


def update_contact(account_id: str, email: str, tenant_id: str) -> dict:
    with _lock:
        record = _record(account_id, tenant_id)
        if not record or record["status"] != "active":
            return {"error": "active account not found", "retryable": False}
        record["email"] = email
        return {"updated": True, "account_id": account_id}


def close(account_id: str, tenant_id: str) -> dict:
    with _lock:
        record = _record(account_id, tenant_id)
        if not record:
            return {"error": "account not found", "retryable": False}
        record["status"] = "closed"
        return {"closed": True, "account_id": account_id}
