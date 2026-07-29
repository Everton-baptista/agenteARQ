# 05. Shim rendering (normative)

A shim is a generated instruction file that an AI assistant reads. Rendering them from one
source is how every assistant ends up following the same rules.

## 1. The core

The core is the concatenation, in filename order, of every `.md` file in `content/core/<lang>/`.

An implementation MUST:

- sort by filename, so ordering is a property of the content rather than of a directory listing
- strip translation front matter before concatenating — it is bookkeeping for validation, and
  leaving it in spends the assistant's context on a hash
- join files with a single newline
- compute the SHA-256 of the **result**, not of the individual files

## 2. The budget

The concatenated core MUST NOT exceed **12288 bytes**. An implementation MUST treat exceeding it
as an error and MUST NOT truncate.

This is the load-bearing rule of the whole layer. What every assistant loads on every session is
capped, so adding an invariant costs something — a rule must be removed or demoted to a
standard. Without a hard budget the core grows until assistants weigh each instruction less, and
the failure is silent.

Per-target budgets apply to the rendered file:

| Target | Path | Budget |
|---|---|---|
| `agents_md` | `AGENTS.md` | 12288 |
| `claude` | `CLAUDE.md` | 12288 |
| `gemini` | `GEMINI.md` | 12288 |
| `qwen` | `QWEN.md` | 12288 |
| `cursor` | `.cursor/rules/agentarch-core.mdc` | 8192 |
| `copilot` | `.github/copilot-instructions.md` | 6144 |
| `windsurf` | `.windsurf/rules/agentarch-core.md` | 6144 |

An implementation MUST fail rather than truncate when a target's budget is exceeded. An
assistant that receives half a rulebook follows half the rules, and nobody finds out until
something goes wrong.

## 3. The header

Every generated file MUST begin with a header — after any format-required front matter —
carrying the content version, the core digest, the target name and the language, and stating
that the file is generated.

```
<!-- agentarch:generated v=<version> core_sha256=<hex> target=<name> lang=<lang>
     DO NOT EDIT. … -->
```

The digest is what makes drift detectable without re-deriving the file. A file with no header
MUST be treated as hand-written, not as stale — the two need different messages.

## 4. One direction only

An implementation MUST NOT read a generated file as a source of rules. There is no merge and no
bidirectional sync.

Once both sides can be authoritative there is no answer to "which one was right", and that
ambiguity is precisely what makes tools in this space drift.

## 5. Custom regions

Content between `<!-- agentarch:custom:start -->` and `<!-- agentarch:custom:end -->` MUST be
preserved across renders.

This is required rather than optional. Without a supported escape hatch, the first legitimate
customisation becomes a fight with the tool, and the team's resolution is to stop running sync —
losing every other guarantee at the same time.

Rendering MUST be idempotent over its own output: rendering a file, then rendering the result,
MUST produce the same bytes. Otherwise every run reports drift forever.

## 6. Drift detection

`sync --check` MUST NOT write. It MUST exit 3 when any target differs, and SHOULD distinguish:

- the file is missing
- the file has no agentarch header, so it was written by hand
- the file was generated from a different core digest

## 7. Derived targets

Some targets are derived from a project artifact rather than rendered from the core. `.mcp.json`
is generated from the MCP allowlist, so the reviewed document is the source and the runtime
config is the derivative.

An implementation MUST NOT apply the core digest header to a target whose format cannot carry a
comment. For those it MUST detect drift by comparing the full generated content.
