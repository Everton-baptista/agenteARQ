---
lang: pt-BR
source: content/standards/en/02-prompt-and-context.md
source_sha256: "976b097fae0718d36932c219de2e6a55475eef7123b91d23eb6a1e79715e8b40"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 02. Prompt e contexto

Propósito: como instrução e dado ficam separados, e por que essa fronteira é estrutural.
Versão: 0.1 · Status: draft

Everything an attacker can influence ends up in the context window. The system prompt is the
only part you author. Keeping that distinction visible in the artifact — rather than trusting it
to hold in a string concatenation — is what this standard is about.

---

## 1. Regras

### control.ai.genai.prompt_versioned

**Intenção.** A prompt change is a behaviour change, and it should be reviewable as one.
**Severidade** `major`, `blocker` from content 1.1.0

`prompts.system` records `path`, a semver `version`, and the file's `sha256`. `AA-REF-004`
re-checks the hash on every run of `validate`, so a prompt edited without a version bump fails
before it ships.

A prompt edited in production during an incident, with no bump, is the most common invisible
change there is — and it silently invalidates every eval taken before it.

### control.ai.genai.untrusted_content_isolation

**Intenção.** Retrieved and received content is data.
**Severidade** `blocker` · **Fail mode** `fail_closed`

It goes in a delimited block, outside the instruction section, and the prompt says that
instructions appearing inside it are evidence of tampering. It is never concatenated into the
system prompt.

`context.rag.untrusted` is `const: true` in the schema. It cannot be turned off, because there
is no deployment in which a retrieved document becomes trustworthy.

### control.ai.genai.output_schema_enforced

**Intenção.** Downstream code receives what it expects.
**Severidade** `major`

Where output feeds a system rather than a person, constrain it — a schema, a grammar, a typed
result. Parsing prose with a regular expression is a guardrail that fails silently.

### control.ai.rag.corpus_versioned and control.ai.rag.citation_required

See `15-supply-chain.md` and `11-evaluation.md`. A corpus that changes underneath a system
invalidates its evals with nothing in the repository to point at, and a confident answer with no
source is a failure presented as a fallback.

---

## 2. Prompt structure

Layers, in this order. The order matters because the last thing the model reads is the thing it
weighs most:

| Layer | Contents |
|---|---|
| Role | who the agent is, in one or two sentences |
| Scope | what it may do, and what it must refuse — mirroring `out_of_scope` |
| Tool policy | when to call what, and what never to infer |
| Refusal policy | how to refuse: what it cannot do, why, and the handoff |
| Output format | shape, length, language, citation requirement |
| Untrusted block | delimited, last, with the tampering warning |

The refusal policy is the layer most often missing. Without it a model that has decided to
refuse will improvise the wording, and the improvisation is where it speculates about what a
human might decide instead.

---

## 3. Variables and trust

`prompts.variables` marks each interpolated value `trusted` or not. Only values your own code
produced are trusted. A `customer_tier` read from the session is trusted; a `question` typed by a
user is not, and neither is anything a tool returned.

The field is not decoration: it is the list of places where untrusted text enters a template,
which is exactly the list a reviewer needs.

---

## 4. Context window and compaction

A long conversation eventually exceeds the window, and something has to be dropped. Whatever
does the dropping is making a safety decision: an injected instruction that survives compaction
while the refusal policy is summarised away has been promoted.

Keep the system prompt intact. Compact the middle of the conversation, never the instructions.

---

## 5. Deve / não deve

| Deve | Não deve |
|---|---|
| Version the prompt and record its hash | Edit it during an incident and move on |
| Put retrieved content in a delimited block | Interpolate it into the system prompt |
| Say instructions inside the block are tampering | Assume the model infers that |
| Mark every variable trusted or not | Interpolate a tool result into a template unmarked |
| Keep instructions out of compaction | Summarise the conversation including the rules |
| Constrain output that feeds a system | Parse prose with a regular expression |

---

## 6. Antipadrões observados

**The prompt that grew.** Nobody can say which paragraph does what, so nobody removes anything,
so it keeps growing and the model weighs each instruction less.

**Retrieved content in the system prompt.** Usually written as a convenience — one template,
one string — and it is the mechanism behind indirect injection.

**The delimiter the model was never told about.** `<context>...</context>` with nothing in the
prompt saying what it means. The tags are decoration unless the instructions give them meaning.

**Compaction that summarises the rules.** The refusal policy gets shortened to "be helpful and
safe" and the injected instruction, being recent, survives verbatim.

**A secret in the prompt.** Every conversation becomes a disclosure channel, and prompts end up
in traces, eval fixtures and bug reports.

---

## Referências externas

Revisado em 2026-07-28. Apenas mapeamentos; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
