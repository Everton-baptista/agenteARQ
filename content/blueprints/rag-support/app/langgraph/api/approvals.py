"""Tenant-scoped, expiring approvals consumed atomically after role authorization."""
from __future__ import annotations

import logging
import secrets
import time
from dataclasses import asdict, dataclass, field

from ..agent.authorization import digest
from ..infra.store import KeyValue, default_store, namespaced

TTL_SECONDS = 15 * 60
audit = logging.getLogger("agent.approval.audit")


@dataclass
class Pending:
    tenant_id: str
    subject: str
    tool: str
    preview: dict
    state: dict
    created_at: float = field(default_factory=time.time)

    @property
    def ttl(self) -> int:
        return min(int(self.preview.get("timeout_s") or TTL_SECONDS), TTL_SECONDS)

    @property
    def expired(self) -> bool:
        return time.time() - self.created_at >= self.ttl


class Queue:
    def __init__(self, store: KeyValue | None = None) -> None:
        self._store = store if store is not None else default_store()

    def put(self, pending: Pending) -> str:
        approval_id = secrets.token_urlsafe(24)
        self._store.put(namespaced(pending.tenant_id, approval_id), asdict(pending), pending.ttl)
        self._audit(approval_id, pending, "pending", pending.subject)
        return approval_id

    def take(self, approval_id: str, tenant_id: str, decision: str, subject: str,
             role: str = "", reason: str = "") -> Pending | None:
        if decision not in ("approve", "deny"):
            return None
        key = namespaced(tenant_id, approval_id)
        raw = self._store.get(key)
        if raw is None:
            return None
        pending = Pending(**raw)
        required = pending.preview.get("approver_role")
        if (pending.tenant_id != tenant_id or pending.expired or not required or role != required):
            return None
        if self._store.take(key, raw) is None:
            return None
        self._audit(approval_id, pending, decision, subject, reason)
        return pending

    @staticmethod
    def _audit(approval_id, pending, decision, subject, reason=""):
        # Hash the reviewed action; do not copy customer arguments or rejection text to logs.
        audit.info("approval decision", extra={
            "request_id": approval_id, "method": "approval", "route": "approval", "status": 0,
            "duration_ms": 0, "tenant": pending.tenant_id, "subject": subject,
            "tool": pending.tool, "decision": decision, "action_hash": digest(pending.preview),
            "reason_hash": digest(reason) if reason else "",
        })

    def __len__(self) -> int:
        return self._store.count()


queue = Queue()
