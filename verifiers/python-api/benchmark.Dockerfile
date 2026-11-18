# Explicit network-enabled preparation for benchmark v2, including real browser tests.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
ENV PATH=/usr/local/bin:/usr/bin:/bin \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    RUFF_CACHE_DIR=/tmp/ruff-cache \
    MYPY_CACHE_DIR=/tmp/mypy-cache \
    XDG_CACHE_HOME=/tmp/cache
WORKDIR /opt/agentarch-verifier
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v python && command -v tar && test -x /bin/sh \
    && python -c 'import sys; assert sys.prefix == sys.base_prefix'

# The arm64 Playwright Node driver needs libatomic before it can install browsers.
RUN apt-get update && apt-get install --no-install-recommends -y libatomic1 \
    && rm -rf /var/lib/apt/lists/*
COPY requirements-benchmark.lock ./
RUN python -m pip install --no-cache-dir --require-hashes --only-binary=:all: -r requirements-benchmark.lock \
    && python -m pip check \
    && python -m ruff --version \
    && python -m mypy --version \
    && python -m pytest --version \
    && python -m bandit --version \
    && python -m playwright --version

# Downloads OS dependencies and the browser matching the pinned Playwright package.
# This network-enabled build step is never run by the offline verification runner.
RUN PLAYWRIGHT_DOWNLOAD_CONNECTION_TIMEOUT=120000 \
    python -m playwright install --with-deps --only-shell chromium \
    && rm -rf /var/lib/apt/lists/*
ENV PIP_NO_INDEX=1
WORKDIR /workspace
