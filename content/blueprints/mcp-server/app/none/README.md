# The application — an agent that acts, with a human on the dangerous part

Three layers, and the dependency direction between them is enforced by `agentarch check`, not by
convention.

```
app/
  api/       transport. May import agent, domain, infra.
    main.py        app factory; middleware order is decided here and commented
    deps.py        caller identity, tenant scope, per-caller budget
    routes.py      thin handlers: validate, resolve who is asking, call the agent, shape the result
    schemas.py     the wire contract — the source of contracts/openapi.json
    middleware.py  access logging with no content in it
    approvals.py   the pending-approval queue
  agent/     the agent. Imports domain and infra. Never api.
    runner.py      the loop, bounded by the manifest
    guardrails.py  the three points — and the role check, which runs before an approval is raised
    tools.py       implementations and the dispatcher
    principal.py   the operator and their role, as the core sees them
    manifest.py    loads the contract and verifies the prompt hash
  domain/    business logic. No LLM, no HTTP.
  infra/     provider client, secrets, storage, telemetry, resilience
  cli.py     the same agent with no web server — proof the layers hold
```

**`agent/` must not import `api/`.** That single rule is why the agent can be run from a test, a
queue worker or the CLI, and it is checked as `control.ai.api.core_transport_separated`. The drift it
prevents is gradual and always locally reasonable: a handler needs one header, so the runner imports
the request, and a few weeks later the agent only runs inside a web server.

## Run it

```bash
python -m venv .venv && source .venv/bin/activate
pip install -r app/requirements.txt
cp .env.example .env          # fill in ANTHROPIC_API_KEY; .env is gitignored

# the service
uvicorn app.api.main:app --reload --env-file .env

# or the same agent with no server at all
python -m app.cli "close the account for acct-4471"
```

Then, in another terminal:

```bash
curl -s localhost:8000/v1/health | jq

curl -s -X POST localhost:8000/v1/ask \
  -H 'content-type: application/json' \
  -H 'authorization: Bearer demo-token-manager' \
  -d '{"instruction":"close the account for acct-4471"}' | jq
```

Four things worth trying, because each one demonstrates a rule rather than a feature:

| try this | what you should see |
|---|---|
| omit the `authorization` header | `401`. Authority comes from the credential; there is no anonymous mode |
| `demo-token-readonly` asking to close an account | refused **before** an approval is raised. A viewer who cannot close accounts must not be able to fill the approver's queue — approval fatigue is manufactured exactly that way |
| `demo-token-support` asking to update a contact | it runs. `write` is reversible, so at L2 it does not stop for anybody |
| `demo-token-manager` asking to close an account | `awaiting_approval`, with the arguments shown. An approver who cannot see them is approving the agent's judgement, and that is the thing in question |
| approve the same id twice | the second is `404`. Replaying an approval replays the irreversible action it authorised |
| add `"role": "admin"` to the body | ignored. The contract has no field for it, so it cannot be overridden |

Watch the log while you do it. It records the route template, status, duration and tenant — and
never the instruction, the reply, the token or any tool result. That is
`capture_content: false` from the manifest being true in the place it usually is not.

## Serve the tools over MCP

The same two tools are also exposed to other agents and IDEs over the Model Context Protocol:

```bash
python -m app.mcp_server
```

stdio only — the client spawns the process, so there is no open port to protect. Point a client
at it with a `.mcp.json` entry:

```json
{
  "mcpServers": {
    "tool-gateway": {
      "command": "python",
      "args": ["-m", "app.mcp_server"],
      "env": { "AGENT_TENANT": "acme", "AGENT_ROLE": "support_agent" }
    }
  }
}
```

Three things to notice, because they are the governance rather than the plumbing:

- **What is advertised is what was reviewed.** `tools/list` and the input schemas are built
  from the `.tool.yaml` specs, so an undeclared tool cannot leak onto the wire.
- **The descriptions are hashed.** `app/mcp_server.py` prints a `descriptions_hash`; a consumer
  pins it in its allowlist, and any edit to a description trips it. That is the rug-pull check
  mcp-consumer demands, honoured on the serving side.
- **Irreversible still pauses.** `notify_customer` over MCP returns `awaiting_approval`, it does
  not send. The human decides through `POST /v1/approvals/{id}` — an MCP call is a transport,
  not an approver.

## Run the tests

```bash
python -m pytest app/tests -q   # no API key needed
```

None of them calls the provider. A suite that needs a credential is a suite that stops running in
CI, and then the rules it covered stop being checked.

## Run the evals

```bash
python evals/run.py --dry-run    # verifies the harness; writes nothing
python evals/run.py              # needs ANTHROPIC_API_KEY; writes a measured result
```

The shipped `evals/results/latest.yaml` says `status: not_run` with null values, and that is
deliberate. An earlier version of this blueprint shipped plausible numbers — groundedness 0.94, a
jailbreak rate of 0.03 — that nobody had measured, and `agentarch conformance` read them and
reported **L3 Proven** for a project one minute old.

So conformance reports L2 until you run the evals, and `agentarch conformance` names this file as
what stands between you and L3. Only `evals/run.py` can write `status: measured`.

## Make it yours

In this order. The order is the point: the manifest is the contract, and changing the code first
means the two disagree with nothing to tell you.

1. **`agentarch/project/agents/tool-gateway/agent.yaml`** — `purpose`, `out_of_scope`, and
   `owner.accountable`, which still names the blueprint's example person. What the agent must refuse
   decides everything else.
2. **The system prompt beside it** — mirror `out_of_scope` into the refusal section, bump the
   version, and run `agentarch validate`, which prints the hash to record.
3. **`app/domain/accounts.py`** — your account operations. Keep them out of `agent/tools.py`: "who
   may close an account" is a rule about your business, and a rule reachable only by asking a
   language model cannot be unit tested, reused in an admin screen, or audited without a token
   budget.
4. **`app/domain/`** — your business rules. Keep them out of `agent/tools.py`: a rule reachable only
   by asking a language model cannot be unit tested, reused in a support screen, or audited without
   a token budget.
5. **`app/api/deps.py`** — replace the demo token dict with your identity provider. What must not
   change is the direction: the tenant comes out of a verified credential, and no request field
   influences it.
6. **`evals/datasets/`** — your cases. Then run them.

`agentarch check` after each step. It is what tells you when the manifest and the code disagree.

## Before production

`deploy/README.md` covers the container, Cloud Run and Kubernetes. Four things there are controls
rather than preferences: the credential comes from a secret store, the container runs as non-root,
`/v1/health` is wired to readiness, and **more than one replica means Redis** — the approval store
defaults to in-memory, so an approval raised by one replica is a 404 at the other and the run is
lost. `app/infra/store.py` has the Redis implementation in full.
