---
lang: pt-BR
source: content/standards/en/14-lifecycle.md
source_sha256: "9d59e75df680cdf7fe70f4696fc37712992ad81e7213af816022a81fefdbc7ea"
translated_at: "2026-07-29"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 14. Ciclo de vida

Propósito: o que invalida a garantia anterior, e o que fazer a respeito.
Versão: 0.1 · Status: draft

Every artifact in this catalogue describes the system at a moment: an eval result, a threat model,
an approval, a review. Certain changes make all of them describe something that no longer exists.

Naming those changes is what this standard does. `agentarch diff --base <ref>` detects them, so
the question at review time is not "did anything important change?" but "here is what changed —
has it been revalidated?"

---

## 1. Regras

### control.ai.lifecycle.revalidation_triggers

**Intenção.** The project decides in advance which changes invalidate its evidence.
**Severidade** `major`

`lifecycle.revalidate_on` lists them. A trigger that fired invalidates prior assurance
regardless of whether anything looks broken.

| Trigger | What it invalidates |
|---|---|
| `model_changed` | every eval, every threat model, every measured behaviour |
| `provider_changed` | the above, plus data handling and failure modes |
| `system_prompt_changed` | the prompt is the behaviour |
| `rag_corpus_changed` | groundedness and citation accuracy |
| `tool_added` | the blast radius the threat model was written against |
| `tool_effect_raised` | the approval matrix |
| `autonomy_raised` | what happens unattended |
| `guardrail_disabled` | whatever it was catching |
| `mcp_server_added` | a new source of authoritative-looking tool descriptions |

`guardrail_disabled` is the one worth watching: it usually arrives as a small edit that makes
something else pass.

### control.ai.lifecycle.review_current

**Intenção.** Nothing runs indefinitely on unchecked assumptions.
**Severidade** `major`

`review_interval_days` and `last_validated_at`. An agent nobody has looked at in a year is
running on assumptions nobody has checked in a year, whether or not anything changed.

### control.ai.lifecycle.rollback_plan

**Intenção.** Reversing a release is a procedure, not an improvisation.
**Severidade** `major`

For an agent, rollback is not only the code: the prompt version, the model id, the corpus
version and the manifest all move together. Reverting the deployment while the corpus stays new
produces a combination that was never evaluated.

---

## 2. What a change does to a release

```
change → agentarch diff --base main
       → triggers fired?  no  → normal review
                          yes → is last_validated_at at or after the change?
                                 yes → proceed
                                 no  → re-run evals, review the threat model,
                                       update last_validated_at
```

`--strict` makes an unanswered trigger exit 6, distinct from a failed gate, so CI can route
"this needs revalidating" differently from "this is unsafe".

---

## 3. Deprecating an agent

Retiring one is not deleting a directory:

1. Announce, with a date.
2. Stop new sessions; let existing ones finish.
3. Delete memory per the retention policy.
4. Revoke credentials and remove tool permissions.
5. Keep the manifest and eval results for the retention period — they are the record of what ran.
6. Remove MCP allowlist entries nothing else uses.

Step 4 is the one that gets skipped, and an agent nobody runs with credentials nobody revoked is
a standing liability.

---

## 4. After an incident

An incident that produced no control change produced no learning. The question is not "what went
wrong" but "which check would have caught this, and does it exist?"

Three honest outcomes: a new control, a severity raised, or a documented decision to accept the
risk with an owner and a review date. "We will be careful" is not one of them.

---

## 5. Deve / não deve

| Deve | Não deve |
|---|---|
| Run `diff` in the pull request | Rely on a reviewer noticing a hash changed |
| Update `last_validated_at` after revalidating | Update it to make the check pass |
| Roll back prompt, model, corpus and code together | Revert the deployment alone |
| Revoke credentials when retiring | Delete the directory and move on |
| Turn an incident into a control | Turn it into a reminder to be careful |

---

## 6. Antipadrões observados

**The date bumped to pass.** `last_validated_at` updated without re-running anything. The
control now measures whether someone edited a field.

**The model upgraded as a chore.** A newer, better model, no revalidation, and the refusal
behaviour changed in a way nobody tested.

**The guardrail removed to unblock a release.** It was producing false positives. The trigger
fired; nobody looked.

**The retired agent with live credentials.** Nothing calls it. Its token still works.

**The partial rollback.** Code reverted, corpus not, and the resulting pair was never evaluated
together.

---

## Referências externas

Revisado em 2026-07-28. Apenas mapeamentos; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
