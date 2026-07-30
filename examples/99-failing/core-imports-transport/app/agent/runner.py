"""The defect: this module reads the request.

Line by line it is reasonable. The loop needed the caller's locale, the request already had it, and
importing `deps` was the shortest path. What it costs is everything the layout was for — this file can
no longer be run from a test, a queue worker, a batch job or a CLI, and every change to the agent now
has to be made through a web server.

The fix is not to pass the request in more cleanly. It is to define the value the loop needs in the
core, and let the transport construct one:

    @dataclass(frozen=True)
    class Principal:
        tenant_id: str
        subject: str
        locale: str = "en"
"""

from fastapi import Request                    # AA-DEP-019
from ..api.deps import current_principal       # AA-DEP-019


def run(request: Request, question: str) -> str:
    principal = current_principal(request.headers.get("authorization", ""))
    locale = request.headers.get("accept-language", "en")
    return f"[{locale}] answer for {principal}"
