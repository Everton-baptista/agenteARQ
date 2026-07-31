"""Storage as a seam: one protocol, an in-memory default, and the shape of a real backend.

The approval queue and the session state both need somewhere to live. In-memory is correct for local
development and wrong for anything with more than one replica — an approval raised by replica A is
invisible to replica B, so the approver gets a 404 and the run is lost.

Stating that as a seam rather than a TODO is the difference between a deployment decision and a
rewrite. `KeyValue` is four methods; the Redis implementation below is fifteen lines, and nothing in
app/api or app/agent changes when you switch.

Two properties the in-memory version has that a real backend must not lose:

  expiry     — `on_timeout: deny` in a tool spec is only true if something actually expires. Set the
               TTL in the store, not in a cleanup job that can be paused.
  namespacing— every key is prefixed with the tenant. It is the last line of defence for isolation
               if a lookup somewhere forgets to check.
"""

from __future__ import annotations

import json
import time
from typing import Any, Protocol


class KeyValue(Protocol):
    """The whole storage interface. Deliberately small.

    A larger interface — queries, indexes, transactions — would push logic into the store and make
    swapping it a project. Everything the agent needs is put, get, delete, and a count for the
    health endpoint.
    """

    def put(self, key: str, value: dict, ttl_seconds: int) -> None: ...

    def get(self, key: str) -> dict | None: ...

    def delete(self, key: str) -> bool: ...

    def count(self) -> int: ...


def namespaced(tenant_id: str, key: str) -> str:
    """Every key carries its tenant.

    Belt and braces: the approval store also checks the tenant explicitly. Two independent
    mechanisms, because cross-tenant leakage is the failure that cannot be walked back.
    """
    return f"{tenant_id}/{key}"


class InMemory:
    """The default. Process-local, lost on restart, wrong for more than one replica.

    Said plainly rather than left to be discovered: with two replicas behind a load balancer, an
    approval raised by one is a 404 at the other. Use Redis or your database in production.
    """

    def __init__(self) -> None:
        self._items: dict[str, tuple[float, dict]] = {}

    def put(self, key: str, value: dict, ttl_seconds: int) -> None:
        self._sweep()
        self._items[key] = (time.time() + ttl_seconds, value)

    def get(self, key: str) -> dict | None:
        self._sweep()
        found = self._items.get(key)
        return found[1] if found else None

    def delete(self, key: str) -> bool:
        return self._items.pop(key, None) is not None

    def count(self) -> int:
        self._sweep()
        return len(self._items)

    def _sweep(self) -> None:
        now = time.time()
        for key in [k for k, (expires, _) in self._items.items() if expires <= now]:
            del self._items[key]


class Redis:
    """A real backend, in full, so the seam is not theoretical.

    Requires `redis>=5` — not in requirements.txt, because the blueprint must run with no server to
    install. Add it when you deploy more than one replica, which is the moment InMemory becomes a
    bug rather than a simplification.

        from redis import Redis as Client
        store = Redis(Client.from_url(os.environ["REDIS_URL"]))
    """

    def __init__(self, client: Any, prefix: str = "agentarch:") -> None:
        self._client = client
        self._prefix = prefix

    def put(self, key: str, value: dict, ttl_seconds: int) -> None:
        # SET with EX rather than SET then EXPIRE: two commands leave a window where a crash
        # between them creates a record that never expires, which is an approval that never denies.
        self._client.set(self._prefix + key, json.dumps(value), ex=ttl_seconds)

    def get(self, key: str) -> dict | None:
        raw = self._client.get(self._prefix + key)
        return json.loads(raw) if raw else None

    def delete(self, key: str) -> bool:
        return bool(self._client.delete(self._prefix + key))

    def count(self) -> int:
        # SCAN, never KEYS: KEYS blocks the server for the length of the keyspace, which is fine
        # in development and an outage in production.
        total = 0
        cursor = 0
        while True:
            cursor, batch = self._client.scan(cursor, match=self._prefix + "*", count=500)
            total += len(batch)
            if cursor == 0:
                return total


def default_store() -> KeyValue:
    """What the service uses unless something else is wired in.

    A function rather than a module-level instance so a test can build its own without inheriting
    state from another test — shared mutable state across tests is a source of failures that only
    appear when the suite runs in a particular order.
    """
    return InMemory()
