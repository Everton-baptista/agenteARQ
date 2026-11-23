# Supply a reviewed Node base by digest (for example node:<version>-bookworm-slim@sha256:...).
# No floating default is provided.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
WORKDIR /opt/agentarch-static-site
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v node && command -v npm && command -v tar && test -x /bin/sh

# The build context holds only this repository's reviewed verifier files; nothing comes from the
# project. Tooling is pinned exactly in package-lock.json and installed here, during the explicit
# network-enabled build, outside /workspace. Lifecycle scripts are disabled.
COPY package.json package-lock.json ./
RUN npm ci --include=dev --ignore-scripts --no-audit --no-fund && rm -rf /root/.npm

# Browser checks (standard 17) are built in only for a product that sets browser_checks: true.
# Without them the agentarch-browser program is absent, so an opted-in check is inconclusive,
# never passed. Playwright installs the Chromium headless shell that matches its pinned version.
ARG WITH_BROWSER=0
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/ms-playwright
COPY agentarch-browser /opt/agentarch-static-site/agentarch-browser
RUN if [ "$WITH_BROWSER" = 1 ]; then \
      ./node_modules/.bin/playwright install --with-deps --only-shell chromium \
      && cp /opt/agentarch-static-site/agentarch-browser /usr/local/bin/agentarch-browser \
      && chmod 0755 /usr/local/bin/agentarch-browser && chmod -R a+rX /opt/ms-playwright \
      && rm -rf /var/lib/apt/lists/* /root/.cache; \
    fi

# The checks' fixed configuration and wrappers. The wrappers ignore configuration in the project.
COPY htmlvalidate.json eslint.config.mjs ./
COPY content/agentarch-links.js /opt/agentarch-content/agentarch-links.js
COPY agentarch-html agentarch-js-lint agentarch-site-links /usr/local/bin/
RUN chmod 0755 /usr/local/bin/agentarch-html /usr/local/bin/agentarch-js-lint /usr/local/bin/agentarch-site-links \
    && chmod -R a+rX /opt/agentarch-static-site /opt/agentarch-content \
    && smoke=$(mktemp -d) && cd "$smoke" \
    && printf '%s\n' '<!doctype html>' '<html lang="en"><head><title>Smoke</title></head>' \
       '<body><main><a href="index.html">Home</a></main><script src="app.js"></script></body></html>' > index.html \
    && printf '%s\n' 'document.title = "ok";' > app.js \
    && agentarch-html && agentarch-js-lint && agentarch-site-links \
    && cd / && rm -rf "$smoke"

# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src

# /workspace (512 MiB) and /tmp (128 MiB) are disposable tmpfs; the root filesystem is read-only.
ENV HOME=/tmp XDG_CACHE_HOME=/tmp/cache npm_config_cache=/tmp/npm-cache npm_config_update_notifier=false
WORKDIR /workspace

# The coordinator replaces the entrypoint; application source arrives through stdin.
