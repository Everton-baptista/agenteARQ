# Two stages, and the final image is scratch.
#
# A governance tool with a shell and a package manager inside it is a tool that can be used for
# something other than governance in a pipeline that trusted it.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /agentarch ./cmd/agentarch

FROM scratch
COPY --from=build /agentarch /agentarch
# Certificates, for `pack add` and `mcp audit --probe` — the only two commands that reach the
# network, both opt-in.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
WORKDIR /work
ENTRYPOINT ["/agentarch"]
