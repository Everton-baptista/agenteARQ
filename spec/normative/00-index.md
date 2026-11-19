# agentarch specification

Version: `spec/1.0.0-draft.1` · Licence: CC BY 4.0

This is the normative half of agentarch. The reference CLI is an implementation of it, not a
definition of it: **where they disagree, this document is right and the code has a bug.**

## Conventions

MUST, MUST NOT, SHOULD, SHOULD NOT and MAY are used in the sense of RFC 2119.

An implementation that satisfies every MUST here and passes `spec/conformance/` may describe
itself as **`agentarch spec/1.0` compliant**. See `07-conformance-levels.md` for what that
claim covers and `TRADEMARK.md` for how the name may be used.

## Documents

| | |
|---|---|
| `01-manifest.md` | the agent manifest and tool spec: fields, resolution of references, what a conforming reader must reject |
| `02-control-and-pack.md` | the format of a control and a pack, and what makes a check valid |
| `03-resolution.md` | how packs are selected and merged into a set of controls with severities |
| `04-expression-language.md` | the restricted language a check is written in |
| `05-shim-rendering.md` | how the core is rendered into assistant instruction files, and how drift is detected |
| `06-exit-codes.md` | what each exit code means, and why they are distinct |
| `07-conformance-levels.md` | L1, L2, L3, expiry, and implementation conformance |
| `08-versioning.md` | the three version lines, compatibility, and what may change in a minor |

## What is deliberately not specified

**How a check is executed against a live system.** agentarch reads artifacts. Anything that
requires running the agent — an eval, a probe of an MCP server — produces an artifact that is
then read. This keeps the standard implementable in any language and keeps it out of the
execution path.

**Which controls a project must adopt.** That is a pack, and packs are data. The spec defines
the format and the resolution, not the content.

**The wording of any message.** An implementation reports findings in its own words. Two
exceptions, both because the message is part of the contract: an unreduced multi MUST tell the
author to use `all` or `any` (`04`), and a rejected expression MUST be an error rather than a
false result.
