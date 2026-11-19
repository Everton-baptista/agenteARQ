# Supply a reviewed Node base by digest (for example node:<version>-bookworm-slim@sha256:...).
# No floating default is provided.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
WORKDIR /opt/agentarch-docs
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v node && command -v npm && command -v tar && test -x /bin/sh

# The build context holds only this repository's reviewed verifier files; nothing comes from the
# project. Tooling is pinned exactly in package-lock.json and installed here, during the explicit
# network-enabled build, outside /workspace. Lifecycle scripts are disabled.
COPY package.json package-lock.json ./
RUN npm ci --include=dev --ignore-scripts --no-audit --no-fund && rm -rf /root/.npm

# The checks' fixed configuration and wrappers. The wrappers ignore configuration in the project.
COPY markdownlint.json ./
COPY content/agentarch-links.js /opt/agentarch-content/agentarch-links.js
COPY agentarch-markdown agentarch-docs-links /usr/local/bin/
RUN chmod 0755 /usr/local/bin/agentarch-markdown /usr/local/bin/agentarch-docs-links \
    && chmod -R a+rX /opt/agentarch-docs /opt/agentarch-content \
    && smoke=$(mktemp -d) && cd "$smoke" \
    && printf '%s\n' '# Smoke' '' 'See [the guide](guide.md).' > README.md \
    && printf '%s\n' '# Guide' '' 'Back to [the start](README.md).' > guide.md \
    && agentarch-markdown && agentarch-docs-links \
    && cd / && rm -rf "$smoke"

# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src

# /workspace (512 MiB) and /tmp (128 MiB) are disposable tmpfs; the root filesystem is read-only.
ENV HOME=/tmp XDG_CACHE_HOME=/tmp/cache npm_config_cache=/tmp/npm-cache npm_config_update_notifier=false
WORKDIR /workspace

# The coordinator replaces the entrypoint; application source arrives through stdin.
