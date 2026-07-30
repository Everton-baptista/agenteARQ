"""The pending-approval queue.

A human approval in a service cannot be an `input()` call. The approver is not in the request, may
take minutes, and may never answer — so the run pauses, its state is parked here, and a second
request carries the decision.

Four properties matter, each mapping to a rule in standard 07:

  expiry        — `on_timeout: deny` is only true if something expires. A pending approval with no
                  deadline is one that gets granted at 3am by whoever is tired enough.
  tenant check  — the decision is authorised against the tenant that raised it, twice: the key is
                  namespaced and the record is checked. Otherwise the approval id is a capability
                  anyone holding it can spend.
  single use    — a decision is consumed. Replaying an approval replays the irreversible action it
                  authorised.
  audit         — who decided what, when, and what they were shown. An approval with no record is
                  indistinguishable from no approval once anyone asks.

Storage is behind infra/store.py, so running this on Redis is configuration rather than a rewrite.
"""

from __future__ import annotations

import logging
import secrets
import time
from dataclasses import asdict, dataclass, field

from ..infra.store import KeyValue, default_store, namespaced

# `on_timeout: deny` in the tool spec is enforced by this number and by the store's TTL.
TTL_SECONDS = 15 * 60

# A separate logger from the access log: this one is an audit trail, it is meant to be retained
# longer, and it deliberately records the tool and the decision but never the message body.
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
    def expired(self) -> bool:
        return time.time() - self.created_at > TTL_SECONDS


class Queue:
    def __init__(self, store: KeyValue | None = None) -> None:
        self._store = store or default_store()

    def put(self, pending: Pending) -> str:
        # A random id, not a counter. A guessable approval id lets somebody approve an action they
        # were never shown.
        approval_id = secrets.token_urlsafe(16)
        self._store.put(
            namespaced(pending.tenant_id, approval_id), asdict(pending), ttl_seconds=TTL_SECONDS
        )
        audit.info(
            "approval requested",
            extra={
                "approval_id": approval_id,
                "tenant": pending.tenant_id,
                "subject": pending.subject,
                "tool": pending.tool,
                "effect": pending.preview.get("effect", "-"),
                "decision": "pending",
            },
        )
        return approval_id

    def take(self, approval_id: str, tenant_id: str, decision: str, subject: str) -> Pending | None:
        """Fetch, consume and record, or return None.

        None covers unknown, expired and wrong-tenant alike. Distinguishing them would confirm that
        an approval exists to somebody not allowed to act on it.
        """
        key = namespaced(tenant_id, approval_id)
        raw = self._store.get(key)
        if raw is None:
            audit.warning(
                "approval decision rejected",
                extra={
                    "approval_id": approval_id,
                    "tenant": tenant_id,
                    "subject": subject,
                    "tool": "-",
                    "effect": "-",
                    "decision": "not_found_or_expired",
                },
            )
            return None

        pending = Pending(**raw)
        if pending.tenant_id != tenant_id or pending.expired:
            return None
        self._store.delete(key)

        audit.info(
            "approval decided",
            extra={
                "approval_id": approval_id,
                "tenant": tenant_id,
                "subject": subject,
                "tool": pending.tool,
                "effect": pending.preview.get("effect", "-"),
                "decision": decision,
            },
        )
        return pending

    def __len__(self) -> int:
        return self._store.count()


queue = Queue()
