"""Where the caller becomes a Principal, and where their budget is counted.

This is the file the standard cares most about in the transport layer, because three controls land
here and nowhere else:

  caller_identified    — the identity comes from a verified credential, not the request body
  budget_per_caller    — a bound per caller, not only per run
  memory scope         — the scope key is derived here and cannot be set by the caller

The auth here is a stand-in: a bearer token looked up in a dict. Replace it with your identity
provider. What must not change is the direction — the tenant comes out of the verified credential,
and no field in the request body can influence it.
"""

from __future__ import annotations

import os
import time
from collections import defaultdict

from fastapi import Depends, Header, HTTPException, status

from ..agent.principal import Principal

# Replace with your identity provider. Kept as a dict so the blueprint runs with no external
# dependency, and named so nobody mistakes it for something to ship.
_DEMO_TOKENS = {
    "demo-token-acme": Principal(
        tenant_id="acme", subject="user-7", customer_id="cus-42", tier="standard"
    ),
    "demo-token-globex": Principal(
        tenant_id="globex", subject="user-3", customer_id="cus-98", tier="priority"
    ),
}


async def current_principal(authorization: str = Header(default="")) -> Principal:
    """Resolve the caller, or refuse.

    Anonymous access is not a mode this service has. An agent that will read a customer's records
    and send them a message has no meaning without knowing who asked.
    """
    scheme, _, token = authorization.partition(" ")
    if scheme.lower() != "bearer" or not token:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="a bearer token is required",
            headers={"WWW-Authenticate": "Bearer"},
        )
    principal = _DEMO_TOKENS.get(token)
    if principal is None:
        # No indication of whether the token is unknown or merely expired. The difference is
        # useful to an attacker and to nobody else.
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="invalid token")
    return principal


# ── per-caller budget ───────────────────────────────────────────────────────────────────────
#
# The manifest bounds one run: max_steps, max_tool_calls, usd_per_run. None of that constrains a
# caller who issues ten thousand runs, and every declared control still passes while the bill
# arrives. So the bound the manifest declares per caller is enforced here, at the only place that
# knows who the caller is.
#
# In-memory and per-process, which is wrong for more than one replica — say so out loud rather
# than letting someone discover it in production. Use Redis, or your gateway's rate limiter.

_RUNS: dict[str, list[float]] = defaultdict(list)
WINDOW_SECONDS = 24 * 60 * 60
DEFAULT_RUNS_PER_DAY = int(os.getenv("AGENT_RUNS_PER_CALLER_PER_DAY", "200"))


def enforce_caller_budget(principal: Principal = Depends(current_principal)) -> Principal:
    now = time.time()
    key = principal.memory_scope_key
    recent = [t for t in _RUNS[key] if now - t < WINDOW_SECONDS]
    if len(recent) >= DEFAULT_RUNS_PER_DAY:
        raise HTTPException(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            detail=f"daily run budget of {DEFAULT_RUNS_PER_DAY} exhausted for this caller",
            headers={"Retry-After": str(WINDOW_SECONDS)},
        )
    recent.append(now)
    _RUNS[key] = recent
    return principal
