# Minimal verifier for the universal profile: the security floor and nothing else.
# Supply a reviewed base by digest that already has the project's toolchain for the checks it
# declares in product.yaml, or build FROM this image and add them. No floating default.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v tar && test -x /bin/sh

# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src
ENV HOME=/tmp XDG_CACHE_HOME=/tmp/cache
WORKDIR /workspace

# The coordinator replaces the entrypoint; application source arrives through stdin.
