"""Optional Redis contract test; runs real Lua semantics through fakeredis, without a server."""
from concurrent.futures import ThreadPoolExecutor
import pytest

fakeredis = pytest.importorskip("fakeredis")
pytest.importorskip("lupa")
from app.infra.store import Redis
from app.api.approvals import Queue, Pending


def test_redis_approval_compare_and_delete_is_atomic():
    backend = Redis(fakeredis.FakeRedis())
    queue = Queue(backend)
    id_ = queue.put(Pending("acme", "requester", "close_account", {"approver_role": "account_manager"}, {}))
    with ThreadPoolExecutor(max_workers=8) as pool:
        results = list(pool.map(lambda _: queue.take(id_, "acme", "approve", "manager", role="account_manager"), range(20)))
    assert sum(value is not None for value in results) == 1
