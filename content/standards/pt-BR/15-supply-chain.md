---
lang: pt-BR
source: content/standards/en/15-supply-chain.md
source_sha256: "6b7fd8d9c2d20ba4acb5370664435c826f72756e32802dece11bd7138c179c0e"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 15. Cadeia de suprimentos

Propósito: saber do que seu agente é feito, e provar que não mudou.
Versão: 0.1 · Status: draft · Escopo: models, prompts, corpora, tools, MCP servers, packages.

An SBOM lists the packages. It does not list the model, the prompt version, the retrieval corpus
or the MCP servers — which is most of what determines how an agent behaves. An **AI-BOM** covers
those, and `agentarch aibom` builds one from the manifests you already maintain.

---

## 1. Regras

### control.ai.supply.model_pinned

**Intenção.** Behaviour cannot change without a diff.
**Severidade** `blocker`

`model.pinned: true` and an immutable identifier. Aliases ending in `latest`, `current` or
`stable` are rejected.

A floating alias means the provider can change your system's behaviour between two runs, with
nothing in your repository to review — and every eval result, threat model and approval taken
before that point silently describes a different system. `model_changed` is a revalidation
trigger for exactly this reason.

### control.ai.supply.dataset_provenance

**Intenção.** Know where your corpus came from and whether you may use it.
**Severidade** `major`

For each corpus: origin, licence, a version or snapshot id, and whether it contains personal
data. `rag_corpus_changed` is a revalidation trigger.

### control.ai.supply.aibom_generated

**Intenção.** One artifact that answers "what is this made of".
**Severidade** `major`

`agentarch aibom` emits models, prompts with their hashes, corpora with their versions, tools
with their effects, MCP servers with their pins, and package dependencies. It is generated from
the manifests, so it cannot drift from them — an AI-BOM maintained by hand is a document about
a system that used to exist.

### control.ai.mcp.server_pinned

See `04-mcp.md`. An MCP server is a supply-chain component that can also write your prompt,
which is why it gets its own standard.

---

## 2. What belongs in an AI-BOM

| Component | Recorded | Why |
|---|---|---|
| Model | provider, immutable id, parameters | behaviour |
| System prompt | path, semver, sha256 | behaviour, and the hash catches an unversioned edit |
| Retrieval corpus | id, version, origin, licence, personal-data flag | behaviour and legal exposure |
| Tool | id, effect, permissions, owner | blast radius |
| MCP server | name, package, exact version, integrity, tool digests | third-party surface |
| Embedding model | provider, id, dimension | changing it silently invalidates an index |
| Packages | the ordinary SBOM | the rest |

---

## 3. Deve / não deve

| Deve | Não deve |
|---|---|
| Pin the model to an immutable id | Use a convenience alias in production |
| Record the prompt's hash in the manifest | Rely on git history to know what shipped |
| Version the corpus and record its licence | Treat "the docs" as a stable input |
| Generate the AI-BOM | Maintain one by hand |
| Sign release artifacts and verify provenance | Trust the artifact because the URL looks right |
| Re-run the gate after any dependency bump | Assume a patch release is behaviour-neutral |

---

## 4. Artefatos e campos afetados

`agent.yaml`: `model.provider`, `model.id`, `model.pinned`, `model.fallback`,
`prompts.system.{path,version,sha256}`, `context.rag.{corpus_id,corpus_version}`,
`lifecycle.revalidate_on`.
`*.tool.yaml`: `effect`, `permissions`, `owner`.
`mcp/allowlist.yaml`: `pin.{package,version,integrity}`, `tool_description_sha256`.
Generated: `ai-bom.json`.

---

## 5. Evidências esperadas

| Control | Evidence |
|---|---|
| `model_pinned` | manifest field |
| `dataset_provenance` | manifest field plus a corpus record |
| `aibom_generated` | the generated `ai-bom.json`, regenerated in CI so it cannot go stale |
| `mcp.server_pinned` | the allowlist plus a passing `mcp audit --probe` |

---

## 6. Antipadrões observados

**The alias that moved.** Quality drops on a Tuesday. Nothing was deployed. The provider rolled
the alias forward, and every eval result predates a different model.

**The prompt edited in production.** A hotfix to the prompt, no version bump, no hash update.
`AA-REF-004` exists because this is the most common invisible change there is.

**The corpus that grew.** Someone added a folder to the retrieval index. Groundedness fell and
the change is nowhere in the repository.

**The embedding model swapped.** New embeddings, old index, cosine distances that no longer mean
anything, and retrieval quality that degrades without an error anywhere.

**The AI-BOM written once for an audit.** Accurate on the day it was produced.

---

## 7. Referências externas

Revisado em 2026-07-28. Mappings only.

- CycloneDX — SBOM format, and the ML extension the AI-BOM output aligns with.
- SLSA — provenance levels for build artifacts.
- OWASP Top 10 for LLM Applications — *Supply Chain*.
- NIST AI RMF 1.0 — *Map 2.3*, on documenting system components and their provenance.
