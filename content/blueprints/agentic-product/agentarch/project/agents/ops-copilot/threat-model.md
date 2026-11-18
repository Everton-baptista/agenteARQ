# Account operations threat model

Scope: development account operations within an authenticated tenant. Account lookup is read,
contact updates are write and account closure is irreversible. The first two operate under the
requester's authority; closure additionally requires the declared account_manager role.

Untrusted inputs: operator requests, model tool arguments, remembered facts and provider output.
The action boundary validates JSON Schema, role and domain limits. Approvals bind the original
principal, call id, tool specification and complete arguments; consumption is atomic and expires.
The resumed batch cannot reset budgets or re-run completed calls. Tenant and subject keys isolate
memory; only action references are remembered. Logs and traces exclude prompt/response content.

Residual limits: the included account adapter and token table are local demonstrations. Production
startup is refused until identity, durable domain storage and shared caller limits are implemented.
Runtime regressions are exercised in app/tests/test_runtime_security.py.
