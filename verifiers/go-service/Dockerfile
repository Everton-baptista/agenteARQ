# Supply a reviewed Go base by digest (for example golang:<version>-bookworm@sha256:...).
# It needs gcc for `go test -race`. No floating default is provided.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG BASE_IMAGE

USER 0:0
WORKDIR /opt/agentarch-verifier
RUN printf '%s\n' "$BASE_IMAGE" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' \
    && command -v go && command -v gcc && command -v tar && test -x /bin/sh

# The build context contains only go.mod and go.sum. Modules are downloaded here, during the
# explicit network-enabled build, into a cache outside /workspace; verification never fetches.
ENV GOMODCACHE=/opt/gomodcache GOFLAGS=-mod=mod GOTOOLCHAIN=local
COPY go.mod go.sum ./
# A module with no dependencies downloads nothing, so create the cache directory first.
RUN mkdir -p /opt/gomodcache && go mod download all && go mod verify && chmod -R a+rX /opt/gomodcache

# Security floor, required by every profile: pinned gitleaks and osv-scanner, fixed configs,
# and the offline OSV database. Rebuild at least every 14 days; an older database is refused.
COPY floor/ /opt/agentarch-floor-src/
RUN sh /opt/agentarch-floor-src/install.sh && rm -rf /opt/agentarch-floor-src

# /workspace (512 MiB) and /tmp (128 MiB) are disposable tmpfs. The build cache must be writable
# and go test -race rebuilds the standard library with race instrumentation, which does not fit
# in /tmp, so the cache lives in the larger /workspace; each check starts with a fresh copy.
ENV GOPROXY=off GOFLAGS=-mod=readonly GOCACHE=/workspace/.cache/go-build GOPATH=/tmp/go CGO_ENABLED=1 \
    HOME=/tmp XDG_CACHE_HOME=/tmp/cache
WORKDIR /workspace

# The coordinator replaces the entrypoint; application source arrives through stdin.
