---
lang: pt-BR
source: content/standards/en/10-privacy.md
source_sha256: "d3e5b49d4b61c3421a3cba464de35b4460fe7cef2b50a57b425cc436baa3620b"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 10. Privacidade

Propósito: dados pessoais em um agente, independente de jurisdição.
Versão: 0.1 · Status: draft

This standard is deliberately jurisdiction-neutral. Legal obligations live in `reg.*` packs, each
declaring its authority and review date, selected by the agent's `jurisdictions`. What is here
are the engineering properties that every one of those regimes assumes, and that no manifest can
be assessed without.

---

## 1. Regras

### control.ai.privacy.capture_content_default_off

**Intenção.** Telemetry does not silently become a copy of every conversation.
**Severidade** `blocker`

`capture_content: false` in telemetry and in evidence. Turning it on is a decision: bounded in
time, with a stated reason and a retention limit.

This is the single easiest way to build a complete, long-lived record of everything users typed,
sitting in systems with different access rules and different retention from the ones that were
assessed. It happens by accepting a default.

### control.ai.privacy.retention_declared

**Intenção.** Something deletes, on a schedule someone chose.
**Severidade** `blocker` where personal data is processed

`retention_days`, per class of data. Then check that something actually deletes — a declared
retention with no job behind it is a sentence in a document.

### control.ai.privacy.redaction_enabled

**Intenção.** Identifiers are removed before they reach storage.
**Severidade** `major`

A redaction profile applied to logs, traces and evidence. Redaction is probabilistic: it
reduces exposure and does not eliminate it, which is why it is `major` and `capture_content` is
`blocker`. Not collecting is stronger than redacting.

### control.ai.privacy.data_categories_declared

**Intenção.** Know what you hold.
**Severidade** `major`

`data_categories` names the classes. Special categories — health, biometric, beliefs, sexual
orientation, criminal records — change the analysis in every regime, so a manifest that does not
distinguish them cannot be assessed.

### control.ai.privacy.memory_no_raw_pii

See `05-memory-and-state.md`.

---

## 2. Where personal data actually accumulates

Not in the database you designed. In:

| Place | How |
|---|---|
| Traces | a span attribute carrying the prompt |
| Eval datasets | real tickets, "just for a realistic test set" |
| Prompt caches | conversation history, kept for latency |
| Model provider logs | depends on the contract, not on your code |
| Memory | see above |
| Incident reports | pasted verbatim into a ticket |

Six copies, one retention policy, one assessment. This is why `capture_content` defaults off
rather than being a setting somebody remembers.

---

## 3. Rights over a system that has memory

A deletion request has to reach every copy above. Design for it before it arrives: a
`scope_key` that identifies a person's memory is what makes deletion mechanically possible, and
retrofitting it after the first request is expensive.

An eval dataset built from real tickets is the copy nobody remembers.

---

## 4. Deve / não deve

| Deve | Não deve |
|---|---|
| Leave `capture_content` off | Turn it on to debug and leave it |
| Synthesise or paraphrase eval data | Build the golden set from real tickets |
| Store a reference | Store the value for convenience |
| Declare retention per class | Declare one number for everything |
| Make deletion reach every copy | Delete from the database and call it done |
| Distinguish special categories | Treat all personal data as one class |

---

## 5. Antipadrões observados

**Content capture enabled for an investigation.** Six months later it is still on and the traces
are the largest store of personal data in the company.

**The golden dataset of real tickets.** Now in git, in CI, on laptops, and in the fixtures a
contributor forked.

**Redaction as the whole answer.** A regex-based redactor that misses formats it was not built
for, protecting a store that did not need to exist.

**Retention declared, nothing deletes.** The number is in the manifest; there is no job.

**Deletion that misses memory.** The record is removed from the database. The agent still recalls
the conversation.

---

## Referências externas

Revisado em 2026-07-28. Apenas mapeamentos; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
