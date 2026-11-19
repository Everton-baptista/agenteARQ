"""Access logging that does not log the thing you are trying to protect.

This is where `capture_content: false` in the manifest either holds or is a lie. A web framework's
default access log records the path, and an application's default error handler records the
exception with whatever was in scope — which for this service is a customer's question, a
retrieved passage, and sometimes their order details.

The failure is not exotic. It is the single most common way personal data ends up in a log
aggregator with a two-year retention and no subject-access path, and it happens on the day the
service ships, silently, because the log looks normal.

What is recorded here: method, route template, status, duration, tenant, and a request id. What is
never recorded: the body, the query string values, the answer, the token, and the exception
message from an unhandled error.
"""

from __future__ import annotations

import logging
import time
import uuid

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

log = logging.getLogger("api.access")

# Header names whose values must never reach a log line. Anything carrying authority belongs here.
REDACT_HEADERS = {"authorization", "cookie", "x-api-key", "proxy-authorization"}


class AccessLog(BaseHTTPMiddleware):
    """One structured line per request, with no content in it."""

    async def dispatch(self, request: Request, call_next):
        request_id = request.headers.get("x-request-id") or uuid.uuid4().hex[:12]
        started = time.perf_counter()

        try:
            response: Response = await call_next(request)
        except Exception:
            # Log that it failed and where — never with the exception text, which routinely
            # contains the value that caused the failure. The traceback goes to your error
            # tracker, which is a system with an owner and a retention policy; the access log is
            # not.
            log.error(
                "request failed",
                extra={
                    "request_id": request_id,
                    "method": request.method,
                    "route": _route_of(request),
                    "status": 500,
                    "duration_ms": round((time.perf_counter() - started) * 1000, 1),
                },
                exc_info=False,
            )
            return JSONResponse(
                status_code=500,
                content={"error": "internal error", "request_id": request_id},
            )

        log.info(
            "request",
            extra={
                "request_id": request_id,
                "method": request.method,
                # The route template, not the concrete path: /v1/approvals/{id} rather than the
                # id itself. Identifiers in a path are personal data in a great many services.
                "route": _route_of(request),
                "status": response.status_code,
                "duration_ms": round((time.perf_counter() - started) * 1000, 1),
                "tenant": getattr(request.state, "tenant_id", "-"),
            },
        )
        response.headers["x-request-id"] = request_id
        return response


def _route_of(request: Request) -> str:
    """The route template, never the concrete path.

    `path_format` is preferred because it keeps `{approval_id}` as a placeholder. An approval id is
    a capability, and identifiers in paths are personal data in a great many services — so logging
    the concrete path is how both end up in a log aggregator with a two-year retention.
    """
    route = request.scope.get("route")
    for attr in ("path_format", "path"):
        value = getattr(route, attr, None)
        if value:
            return value
    # No route means the request never matched one — a 404. Log the method only; an unmatched path
    # is attacker-controlled input and does not belong in a log line.
    return "(unmatched)"


def redact_headers(headers: dict) -> dict:
    """For anywhere a request is reproduced — a debug endpoint, a captured trace, a bug report."""
    return {
        k: ("[redacted]" if k.lower() in REDACT_HEADERS else v) for k, v in headers.items()
    }
