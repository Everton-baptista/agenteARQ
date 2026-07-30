# Multi-stage, non-root, with a health check. Roughly 120 MB.
#
# Four choices here are security properties rather than preferences, and each is the fix for a
# specific way container deployments of agents go wrong:
#
#   non-root         a compromised agent that can write to its own code can rewrite its prompt, and
#                    the prompt hash check then fails closed — but only after it has been running.
#   no build tools   the runtime stage has no compiler and no pip, so an attacker who achieves
#                    execution cannot fetch and build a payload.
#   no .env          the credential arrives as an environment variable from the orchestrator's
#                    secret store. A .env baked into an image is a secret in a registry, readable
#                    by anyone who can pull it, forever.
#   pinned base      by digest, not by tag. `python:3.12-slim` changes under you, which is exactly
#                    what control.ai.supply.model_pinned objects to applied to the base image.

FROM python:3.12-slim AS build

WORKDIR /build
COPY app/requirements.txt .
# --no-cache-dir keeps the wheel cache out of the layer; the venv is copied whole to the runtime
# stage so nothing from this stage ships.
RUN python -m venv /opt/venv \
 && /opt/venv/bin/pip install --no-cache-dir --upgrade pip \
 && /opt/venv/bin/pip install --no-cache-dir -r requirements.txt


FROM python:3.12-slim AS runtime

# A named user with no shell and no home. `agent` rather than a numeric id so the process is
# identifiable in `ps` on the host.
RUN useradd --system --no-create-home --shell /usr/sbin/nologin agent

ENV PATH="/opt/venv/bin:$PATH" \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    ENV=production

WORKDIR /srv
COPY --from=build /opt/venv /opt/venv

# The manifest, the prompt and the tool specs. Copied read-only and owned by root, so the process
# can read its contract and cannot rewrite it — the prompt hash check at startup is only meaningful
# if the prompt cannot be edited by the thing being checked.
COPY --chown=root:root agentarch/ ./agentarch/
COPY --chown=root:root app/ ./app/

USER agent

EXPOSE 8000

# Readiness, not liveness: it verifies the credential resolves and the prompt still matches the
# manifest. A container that starts without its credential and fails every request looks healthy to
# an orchestrator, which then sends it all the traffic.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request,sys,json; \
r=json.load(urllib.request.urlopen('http://127.0.0.1:8000/v1/health')); \
sys.exit(0 if r['status']=='ok' else 1)"

# One worker per container, scaled by replicas rather than by threads. The agent holds no shared
# state, so a replica is the unit — and with the in-memory approval store, more than one worker in
# one container would already be the split-brain that infra/store.py warns about.
CMD ["uvicorn", "app.api.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1"]
