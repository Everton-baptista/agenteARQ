# Supply a reviewed global-Python base by digest. No floating default is provided.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
ENV PATH=/usr/local/bin:/usr/bin:/bin \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    RUFF_CACHE_DIR=/tmp/ruff-cache \
    MYPY_CACHE_DIR=/tmp/mypy-cache \
    XDG_CACHE_HOME=/tmp/cache
WORKDIR /opt/agentarch-verifier
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v python && command -v tar && test -x /bin/sh \
    && python -c 'import sys; assert sys.prefix == sys.base_prefix, "Use a global Python base, not a project virtualenv"'

# Include runtime dependencies and Ruff, mypy, pytest and Bandit, all pinned with hashes.
COPY requirements-verifier.lock ./
# Wheels only: building source distributions is a separate reviewed supply-chain step.
# This installs globally, outside /workspace and its disposable tmpfs overlay.
RUN python -m pip install --no-cache-dir --require-hashes --only-binary=:all: -r requirements-verifier.lock \
    && python -m pip check \
    && python -m ruff --version \
    && python -m mypy --version \
    && python -m pytest --version \
    && python -m bandit --version
# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src

ENV PIP_NO_INDEX=1
WORKDIR /workspace

# The coordinator replaces the entrypoint; application source arrives through stdin.
