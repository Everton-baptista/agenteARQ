# The application — an agent using servers it did not write

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
    runner.py      the loop; checks the server handshake before the first model call
    guardrails.py  the three points. No citation rule: there is no passage id to check against
    principal.py   who the caller is, as the core sees them
    manifest.py    loads the contract and verifies the prompt hash
  infra/     provider, secrets, storage, telemetry, resilience — and mcp.py, the allowlisted client
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
python -m app.cli "what does the documentation say about rollbacks?"
```

Then, in another terminal:

```bash
curl -s localhost:8000/v1/health | jq

curl -s -X POST localhost:8000/v1/ask \
  -H 'content-type: application/json' \
  -H 'authorization: Bearer demo-token-acme' \
  -d '{"question":"what does the documentation say about rollbacks?"}' | jq
```

Four things worth trying, because each one demonstrates a rule rather than a feature:

| try this | what you should see |
|---|---|
| `curl /v1/health` | `mcp_verified: true` — every allowlisted server is reachable **and** still serving the tool descriptions that were reviewed |
| change a tool's description on the server, then restart | startup fails with a rug pull, naming both digests. The artifact you approved and the artifact serving traffic have diverged, and the fix is a human reading the new description |
| set `default: allow` in the allowlist | the service refuses to start. An allowlist that does not default to deny is a list of suggestions |
| remove a tool from `tools_allow` | the model can still see the server, and the call is refused. What a server advertises is a proposal; the allowlist is the contract |
| point `servers_used` at a server the project allowlists but this agent does not declare | refused. Allowlisted for the project is not available to every agent in it |

Watch the log while you do it. It records the route template, status, duration and tenant — and
never the request, the reply, the token or any tool result. That is
`capture_content: false` from the manifest being true in the place it usually is not.

## Run the tests

```bash
pytest app/tests -q      # no API key needed
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

1. **`agentarch/project/agents/docs-assistant/agent.yaml`** — `purpose`, `out_of_scope`, and
   `owner.accountable`, which still names the blueprint's example person. What the agent must refuse
   decides everything else.
2. **The system prompt beside it** — mirror `out_of_scope` into the refusal section, bump the
   version, and run `agentarch validate`, which prints the hash to record.
3. **`agentarch/project/mcp/allowlist.yaml`** — your servers. Pin each to an exact version, list
   the tools you reviewed, and record a `tool_description_sha256` per tool. `latest` means the code
   you reviewed and the code you run are different artifacts sharing a name.
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
