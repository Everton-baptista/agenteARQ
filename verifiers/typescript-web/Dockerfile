# Supply a reviewed Node/Playwright base by digest. No floating default is provided.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
WORKDIR /opt/agentarch-verifier
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v node && command -v npm && command -v tar && test -x /bin/sh

# The build context contains only these two reviewed dependency inputs.
COPY package.json package-lock.json ./
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
# Network access happens during this explicit image build, never during verification.
# Lifecycle scripts are disabled. Dependencies requiring a native build need a reviewed,
# package-specific build step here; do not silently enable every install script.
RUN node -e 'const p=require("./package.json"); if(p.workspaces) throw new Error("This recipe requires a single package; adapt workspace dependency installation explicitly")' \
    && npm ci --include=dev --ignore-scripts --no-audit --no-fund \
    && test ! -e /node_modules \
    && mv node_modules /node_modules

# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src

# /workspace is overlaid by tmpfs on every check. Dependencies must live outside it.
# Node finds /node_modules by walking up from /workspace; npm scripts also need its bins.
ENV PATH="/node_modules/.bin:${PATH}" \
    npm_config_cache=/tmp/npm-cache \
    npm_config_offline=true \
    npm_config_audit=false \
    npm_config_fund=false \
    npm_config_update_notifier=false \
    XDG_CACHE_HOME=/tmp/cache
WORKDIR /workspace

# The coordinator replaces the entrypoint and passes each profile command directly.
# Browser binaries and their OS libraries must already exist in the selected base.
