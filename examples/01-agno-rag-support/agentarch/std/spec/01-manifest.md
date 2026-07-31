# 01. Manifest and tool spec (normative)

The manifest is the contract. If behaviour and manifest disagree, one of them is a bug — and an
implementation's job is to make that disagreement visible, not to reconcile it.

## 1. Files and locations

| Artifact | Location | Schema |
|---|---|---|
| agent manifest | `agentarch/project/agents/<id>/agent.yaml` | `agent.manifest.schema.json` |
| tool spec | `agentarch/project/tools/<id>.tool.yaml` | `tool.spec.schema.json` |
| MCP allowlist | `agentarch/project/mcp/allowlist.yaml` | `mcp.allowlist.schema.json` |
| waivers | `agentarch/project/waivers.yaml` | `waivers.schema.json` |
| baseline | `agentarch/project/baseline.json` | `baseline.schema.json` |
| configuration | `agentarch/agentarch.yaml` | `agentarch.config.schema.json` |

An implementation MUST discover agents by directory, not by a list. A manifest that exists is an
agent; there is no registration step to forget.

## 2. Parsing

YAML 1.2. An implementation MUST convert to the JSON data model before validating, and MUST NOT
accept a document whose top level is not a mapping.

Two YAML behaviours cause real defects and an implementation MUST NOT paper over them:

- An unquoted scalar containing `: ` parses as a mapping. This is a document error; report it.
- A long digit string parses as a number. `sha256: 0000…` is an integer, not a hash, and the
  schema will reject it. Report the type, not a generic failure.

## 3. Reference resolution

`tools[].ref` and `mcp.allowlist_ref` are **relative to the agent's own directory**. An
implementation MUST NOT resolve them relative to the working directory or the project root.

A reference that does not resolve MUST be reported (`AA-REF-002`) and the referenced document
MUST be treated as absent rather than as empty. A tool whose spec could not be read makes tool
controls fail; it MUST NOT make them pass.

## 4. The prompt hash

`prompts.system.sha256` MUST be the SHA-256 of the file at `prompts.system.path`, byte for byte,
with no normalisation.

An implementation MUST verify it on every validation (`AA-REF-004`) and MUST report a mismatch
as an error rather than updating the field. Updating it silently would defeat the purpose: the
check exists to catch a prompt edited without a version bump, which is an invisible behaviour
change that invalidates every eval taken before it.

The reported message SHOULD include both digests, truncated, and SHOULD say that the version
needs bumping.

## 5. Fields with non-obvious semantics

**`out_of_scope`** MUST have at least one entry. An empty list is not "nothing is out of scope";
it is a manifest that has not been thought about.

**`owner.accountable`** is a person. An implementation cannot verify that, and MUST NOT try — a
regex for "looks like a name" would reject real people. It MUST require the field to be
non-empty and SHOULD say in the message that a team alias does not satisfy it.

**`context.rag.untrusted`** MUST be `true` where present. The schema pins it as a constant.
There is no deployment in which a retrieved document becomes trustworthy, and making it
configurable would invite exactly one wrong configuration.

**`autonomy.level`** describes the deployment as it is, not as intended. An implementation has
no way to check this and MUST NOT pretend otherwise; `07-hitl.md` in the content explains the
consequence.

**`guardrails`** MUST have all three keys — `input`, `output`, `action`. An empty array is a
recorded decision; a missing key is an oversight, and the schema distinguishes them on purpose.

**`jurisdictions`** selects which `reg.*` packs apply. An implementation MUST treat an absent or
empty list as "no jurisdiction packs apply", never as "all of them".

## 6. Identifiers

Agent ids are lowercase kebab-case; tool ids are lowercase snake_case. Both MUST match in every
language — a translated manifest does not translate its ids, so error messages, searches and
cross-references stay interoperable between teams.

Ids MUST be unique within a project across agents, and across tools (`AA-DUP-006`).

## 7. Unknown fields

The schemas set `additionalProperties: false`. An implementation MUST reject an unknown field
rather than ignoring it.

This is stricter than most tools and deliberate: a typo in a security-relevant field name that
is silently ignored produces a manifest that reads as if the control were configured. `egres`
instead of `egress` must not be a silent no-op.

## 8. What a conforming implementation must reject

- a document that is not a mapping, or has no `agent`/`tool` key
- an unknown field at any level
- `schema_version` with an unsupported major (exit 1, not exit 2 — see `08-versioning.md`)
- a prompt hash that does not match
- a reference that does not resolve
- duplicate ids
