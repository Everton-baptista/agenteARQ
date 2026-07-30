# Deploying this

Three shapes, in order of how much they ask of you. All three run the same image; the differences
are where the credential comes from and who restarts the process.

Whichever you pick, four things must be true, and each of them is a control rather than a
preference:

| | why |
|---|---|
| the credential arrives from a secret store, never in the image or a `.env` | `control.ai.api.secrets_not_committed` — a `.env` baked into an image is a secret in a registry, readable by anyone who can pull it, forever |
| the container runs as a non-root user | a process that can write to its own prompt can change its behaviour, and the hash check then fails only after it has already been serving |
| `/v1/health` is wired to readiness, not just liveness | it verifies the credential resolves *and* the prompt still matches the manifest; a container serving an unreviewed prompt should not receive traffic |
| more than one replica means Redis | the approval store defaults to in-memory, so an approval raised by replica A is a 404 at replica B — and a handoff paused mid-chain is lost with it. See `app/infra/store.py` |

One more, specific to this blueprint: the two agents share one step budget (`app/agent/budget.py`)
and one run. A deploy that scales by running the orchestrator and the specialist as separate
services breaks that invariant — the budget, the cycle detection and the approval chain all assume
one process owns the whole handoff. Scale this service horizontally as a unit, or split the agents
deliberately and redesign the budget; do not drift into the split.

---

## Local — docker compose

```bash
cp .env.example .env      # fill in ANTHROPIC_API_KEY; .env is gitignored
docker compose up --build
curl -s localhost:8000/v1/health | jq
```

`compose.yaml` reads `.env` through `env_file`, which keeps the value out of the image and out of
`docker inspect`'s command line. It also starts a Redis, so the multi-replica path is exercised
locally rather than discovered in production.

## Cloud Run

```bash
PROJECT=your-project
REGION=us-central1

gcloud secrets create ANTHROPIC_API_KEY --data-file=- <<< "$YOUR_KEY"

gcloud run deploy multi-agent-handoff \
  --source . \
  --region "$REGION" \
  --no-allow-unauthenticated \
  --set-secrets ANTHROPIC_API_KEY=ANTHROPIC_API_KEY:latest \
  --set-env-vars ENV=production,OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
  --concurrency 8 \
  --max-instances 10
```

`--no-allow-unauthenticated` is the default worth keeping: the service authenticates its own callers
with bearer tokens, and IAM in front of that is a second lock rather than a redundant one.

`--max-instances` is a cost bound. An agent under load can autoscale into a bill that nobody
approved, and the per-caller budget in `app/api/deps.py` limits one caller rather than the total.

Two Cloud Run properties to know: instances are frozen between requests, so background work does not
continue after a response; and with more than one instance the in-memory approval store breaks. Set
`REDIS_URL` and switch the store.

## Kubernetes

`deploy/k8s.yaml` is a Deployment, a Service and a HorizontalPodAutoscaler. The parts that matter:

- the credential is a `secretKeyRef`, never an env literal in the manifest
- `readinessProbe` on `/v1/health` and a separate, cheaper `livenessProbe`, so a service that is
  temporarily unready — provider circuit open — stops receiving traffic without being killed and
  restarted, which would lose every pending approval
- `securityContext` with `runAsNonRoot`, `readOnlyRootFilesystem` and all capabilities dropped
- `resources.limits`, because an agent that retries under memory pressure gets OOM-killed mid-run
  and the caller sees a connection reset rather than an error

```bash
kubectl create secret generic multi-agent-handoff --from-literal=ANTHROPIC_API_KEY="$YOUR_KEY"
kubectl apply -f deploy/k8s.yaml
```

---

## What to watch

The service emits OpenTelemetry spans and three metrics (`app/infra/telemetry.py`). The four things
worth alerting on:

| signal | why this one |
|---|---|
| `gen_ai.client.cost` by tenant, rate of change | two agents per request means the bill per request is not the model price you budgeted; the rate of change is the signal, not the absolute |
| handoffs per run, and runs ended by the shared budget | a rise means the agents are bouncing work back and forth — cycle detection is catching something, or the budget is miscalibrated for real questions |
| `provider_circuit` on `/v1/health` | open means every request is failing fast. Fast, but failing |
| pending approvals, and their age | approvals that expire unanswered are work silently dropped. A rising age means the approver queue has nobody watching it |

Do not alert on p99 latency alone. An agent that pauses for human approval has a legitimately
unbounded response time, and paging somebody for it teaches them to ignore the page.
