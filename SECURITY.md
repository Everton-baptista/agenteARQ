# Security policy

## Reporting

Report privately through GitHub Security Advisories on this repository. Do not open a public
issue.

Include what you did, what happened, and what you expected. A proof of concept helps; a working
exploit is not required.

## Priority

One class comes before everything else: **anything that lets a pack execute code.**

Packs are data that travels — through the registry, from third parties, into build pipelines
that run with credentials. The entire trust model rests on them being inert. A pack that can
reach the host, the network, or the filesystem is a supply-chain vulnerability in every project
that installed it, and `spec/conformance/expr` treats rejecting that as a conformance
requirement rather than an implementation detail.

Also high priority:

- the `--probe` path in `mcp audit` leaking environment variables to a probed server
- an expression that terminates the process, exhausts memory, or does not terminate
- generated output that lets a hand edit pass `sync --check`

## Scope

In scope: the CLI, the expression language, the schemas, the generated output.

Out of scope: vulnerabilities in an MCP server you allowlisted — that is what
`04-mcp.md` and `mcp audit` exist to help you find, and reporting them here does not reach the
people who can fix them.

## Supported versions

While `spec/1.x` is current, fixes land on the latest minor. There is no long-term-support
branch and pretending otherwise would be misleading.
