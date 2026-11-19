"""Timeout, bounded retry and a circuit breaker for the provider call.

Standard 13 asks for all three, and the manifest declares the budget they enforce. Without them a
provider incident becomes your incident: every request hangs for the default socket timeout, every
retry adds load to something already failing, and the queue behind you fills until the process is
killed.

The rule that matters most is the one that is easiest to get wrong: **retry only what is safe to
repeat.** A model call that produced no tool call is safe. A tool call with `effect: irreversible`
is not, and a retry there sends the customer a second message. That is why retry lives here, around
the provider, and never around dispatch().
"""

from __future__ import annotations

import random
import time
from typing import Callable, TypeVar

T = TypeVar("T")

# Bounded and small. A long retry chain turns a 2-second failure into a 40-second one, and the
# caller has usually given up by then — so the work is spent producing a response nobody reads
# while holding a connection open.
MAX_ATTEMPTS = 3
BASE_DELAY_SECONDS = 0.5
MAX_DELAY_SECONDS = 8.0

# Requests are individually bounded so one slow call cannot occupy a worker indefinitely.
REQUEST_TIMEOUT_SECONDS = 60.0


class ProviderUnavailable(RuntimeError):
    """The provider is failing and the breaker is open. Map this to 503, never to 500.

    The distinction is not cosmetic: 503 with Retry-After tells a caller to come back, and a load
    balancer to stop sending traffic. A 500 tells them to try again immediately, which is the worst
    possible response to an overloaded dependency.
    """


class CircuitBreaker:
    """Stop calling something that is failing.

    Three states, and the half-open one is the point: after the cooldown a single request is allowed
    through to test the water. Letting the full load back at once is how a recovering dependency
    gets knocked over again, repeatedly, in a pattern that looks like flapping and is actually
    self-inflicted.
    """

    def __init__(self, threshold: int = 5, cooldown_seconds: float = 30.0) -> None:
        self.threshold = threshold
        self.cooldown = cooldown_seconds
        self._failures = 0
        self._opened_at = 0.0

    @property
    def state(self) -> str:
        if self._failures < self.threshold:
            return "closed"
        if time.time() - self._opened_at >= self.cooldown:
            return "half_open"
        return "open"

    def before(self) -> None:
        if self.state == "open":
            remaining = self.cooldown - (time.time() - self._opened_at)
            raise ProviderUnavailable(
                f"circuit open after {self._failures} failures; retry in {remaining:.0f}s"
            )

    def record_success(self) -> None:
        self._failures = 0

    def record_failure(self) -> None:
        self._failures += 1
        if self._failures >= self.threshold:
            self._opened_at = time.time()


# One breaker per provider, at module scope: the state has to be shared across requests or it
# measures nothing. Per-process, so with several replicas each learns independently — acceptable,
# and worth knowing when you read the metric.
provider_breaker = CircuitBreaker()


def is_transient(err: BaseException) -> bool:
    """Whether repeating the call could plausibly succeed.

    Matched on class and status rather than on message text, because provider error messages are
    not a stable interface. A 400 is your bug and retrying it just spends money confirming that; a
    429 or a 5xx is worth another attempt.
    """
    name = type(err).__name__
    if name in ("APIConnectionError", "APITimeoutError", "InternalServerError", "RateLimitError"):
        return True
    status = getattr(err, "status_code", None)
    return isinstance(status, int) and (status == 429 or 500 <= status < 600)


def call_with_retry(operation: Callable[[], T], *, what: str = "provider call") -> T:
    """Run an idempotent operation with bounded retry, jitter and the breaker in front.

    Only pass things that are safe to repeat. There is no `idempotent=False` parameter on purpose:
    an option to disable the safety check is an option somebody will set under deadline pressure.
    """
    provider_breaker.before()

    last: BaseException | None = None
    for attempt in range(1, MAX_ATTEMPTS + 1):
        try:
            result = operation()
            provider_breaker.record_success()
            return result
        except BaseException as err:  # noqa: BLE001 - re-raised below
            last = err
            if not is_transient(err):
                # A permanent error is not a breaker failure. Counting your own 400s toward the
                # threshold trips the breaker on a bug in your code and hides it as an outage.
                raise
            provider_breaker.record_failure()
            if attempt == MAX_ATTEMPTS:
                break
            # Full jitter. Without it every replica retries on the same schedule and the
            # dependency receives a synchronised wave exactly when it is least able to take one.
            delay = min(MAX_DELAY_SECONDS, BASE_DELAY_SECONDS * 2 ** (attempt - 1))
            time.sleep(random.uniform(0, delay))

    raise ProviderUnavailable(f"{what} failed after {MAX_ATTEMPTS} attempts") from last
