"""The model provider client: credential, timeout and the retry boundary.

One module touches the credential, and it gets the value from infra/secrets.py rather than reading
the environment itself — so swapping to a secret manager is a change there and nothing here.

The retry boundary is here rather than in the runner for a reason worth stating: what is safe to
repeat is a property of the call, and only this layer knows that a model call which produced no tool
use can be repeated while a tool call that sent a message cannot.
"""

from __future__ import annotations

import functools

from anthropic import Anthropic

from .resilience import REQUEST_TIMEOUT_SECONDS, call_with_retry
from .secrets import SecretNotFound, resolve

CREDENTIAL_NAME = "ANTHROPIC_API_KEY"


@functools.lru_cache(maxsize=1)
def model_client() -> Anthropic:
    """One client for the process.

    Cached because the client holds a connection pool: constructing one per request works and then
    quietly becomes the reason p99 latency is bad under load.
    """
    return Anthropic(
        api_key=resolve(CREDENTIAL_NAME),
        # An explicit timeout, because the SDK default is generous enough that a hung call occupies
        # a worker long past the point where the caller has given up.
        timeout=REQUEST_TIMEOUT_SECONDS,
        # Retries are handled by call_with_retry, which shares a circuit breaker with everything
        # else. Two independent retry layers multiply into nine attempts nobody asked for.
        max_retries=0,
    )


def create_message(**kwargs):
    """One provider call, with retry, jitter and the breaker in front.

    Safe to repeat: the call has no side effect beyond cost, and a retried call that produces a
    tool_use has not performed it — dispatch does, and dispatch is never retried.
    """
    client = model_client()
    return call_with_retry(lambda: client.messages.create(**kwargs), what="messages.create")


def credential_present() -> bool:
    """For the readiness probe.

    A service that starts without its credential and fails every request looks healthy to a load
    balancer, which then sends it all the traffic.
    """
    try:
        resolve(CREDENTIAL_NAME)
        return True
    except SecretNotFound:
        return False


# Kept as an alias so a caller that only wants to report a missing credential does not have to
# import the exception from two modules.
MissingCredential = SecretNotFound
