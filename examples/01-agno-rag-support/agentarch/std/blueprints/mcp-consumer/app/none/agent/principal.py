"""Who the caller is — and why the agent core defines this rather than the API layer.

The agent needs an identity to scope memory, to resolve which customer's orders it may read, and
to attribute cost. It does not need to know that the identity arrived in a bearer token. So the
type lives here, in the core, and the transport layer's only job is to construct one.

That direction matters. If this class held a reference to the request, every tool would be one
attribute away from reading a header, and the core would stop being runnable from a queue worker,
a test, or a CLI.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Principal:
    """The authenticated caller, resolved server-side.

    Frozen on purpose. A mutable identity is one that something downstream can rewrite, and the
    single most valuable thing an injected instruction can achieve is to change who the agent
    thinks it is acting for.
    """

    tenant_id: str      # the isolation boundary; every memory and data read is scoped by this
    subject: str        # the end user or service account
    customer_id: str    # which customer's records this caller may see
    tier: str = "standard"

    @property
    def memory_scope_key(self) -> str:
        """The value for the manifest's `context.memory.scope_key`.

        Derived from the token, never accepted from the request body. A scope key the caller can
        set is not a scope — it is a parameter, and two tenants end up sharing one store while
        every declared control still passes.
        """
        return f"{self.tenant_id}:{self.subject}"
