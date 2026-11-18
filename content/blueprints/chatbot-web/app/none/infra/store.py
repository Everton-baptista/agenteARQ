"""Bounded key-value storage with atomic conditional consumption.

The in-memory adapter is for one development process. Redis uses a compare-and-delete script,
so two workers cannot consume one approval. Neither adapter returns mutable stored references.
"""
from __future__ import annotations

import copy
import json
import os
import threading
import time
from typing import Any, Protocol


def _encode(value: dict) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"))


class KeyValue(Protocol):
    def put(self, key: str, value: dict, ttl_seconds: int) -> None: ...
    def get(self, key: str) -> dict | None: ...
    def delete(self, key: str) -> bool: ...
    def take(self, key: str, expected: dict) -> dict | None: ...
    def count(self) -> int: ...


def namespaced(tenant_id: str, key: str) -> str:
    # Length-prefix prevents a tenant containing a slash from colliding with another namespace.
    return f"{len(tenant_id)}:{tenant_id}/{key}"


class InMemory:
    def __init__(self) -> None:
        self._items: dict[str, tuple[float, dict]] = {}
        self._lock = threading.RLock()

    def put(self, key: str, value: dict, ttl_seconds: int) -> None:
        with self._lock:
            self._sweep()
            self._items[key] = (time.time() + ttl_seconds, copy.deepcopy(value))

    def get(self, key: str) -> dict | None:
        with self._lock:
            self._sweep()
            found = self._items.get(key)
            return copy.deepcopy(found[1]) if found else None

    def delete(self, key: str) -> bool:
        with self._lock:
            return self._items.pop(key, None) is not None

    def take(self, key: str, expected: dict) -> dict | None:
        with self._lock:
            current = self.get(key)
            if current is None or current != expected:
                return None
            self._items.pop(key)
            return current

    def count(self) -> int:
        with self._lock:
            self._sweep()
            return len(self._items)

    def _sweep(self) -> None:
        now = time.time()
        for key in [k for k, (expires, _) in self._items.items() if expires <= now]:
            del self._items[key]


class Redis:
    def __init__(self, client: Any, prefix: str = "agentarch:") -> None:
        self._client, self._prefix = client, prefix

    def put(self, key: str, value: dict, ttl_seconds: int) -> None:
        if ttl_seconds <= 0:
            self.delete(key)
            return
        self._client.set(self._prefix + key, _encode(value), ex=ttl_seconds)

    def get(self, key: str) -> dict | None:
        raw = self._client.get(self._prefix + key)
        return json.loads(raw) if raw else None

    def delete(self, key: str) -> bool:
        return bool(self._client.delete(self._prefix + key))

    def take(self, key: str, expected: dict) -> dict | None:
        raw = self._client.eval(
            "local v=redis.call('GET',KEYS[1]); "
            "if v and v==ARGV[1] then redis.call('DEL',KEYS[1]); return v end; return nil",
            1, self._prefix + key, _encode(expected),
        )
        return json.loads(raw) if raw else None

    def count(self) -> int:
        return sum(1 for _ in self._client.scan_iter(match=self._prefix + "*", count=500))


def default_store() -> KeyValue:
    mode = os.getenv("AGENT_STORE", "memory")
    if mode == "redis":
        from redis import Redis as Client
        url = os.environ["REDIS_URL"]
        return Redis(Client.from_url(url, socket_timeout=3, socket_connect_timeout=3))
    if mode != "memory":
        raise ValueError("AGENT_STORE must be memory or redis")
    if os.getenv("ENV", "dev") == "production":
        raise RuntimeError("in-memory storage is disabled in production")
    return InMemory()
