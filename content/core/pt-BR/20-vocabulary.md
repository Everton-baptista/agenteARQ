---
lang: pt-BR
source: content/core/en/20-vocabulary.md
source_sha256: "3ef6c959ebf58ec9871adb6e8b9afee1c37b6109e86d0e99db2ab8b2a26d43a2"
translated_at: "2026-07-30"
translators: ["everton"]
---

# Vocabulário

Glossário completo: `agentarch/std/standards/00-index.md`. Os termos das invariantes:

- **Effect** — o que a tool faz ao mundo: `read`, `write`, `irreversible`, `money`,
  `communication`. Determina aprovação e guardrails.
- **Guardrail** — verificação no **input** do usuário, **output** do modelo ou **ação** de tool.
  Não é instrução no prompt.
- **Fail mode** — quando um guardrail não decide: `fail_closed` (bloqueia), `fail_warn`
  (registra), `fail_open` (permite).
- **Conteúdo não confiável** — tudo que não foi escrito pelo seu código. Sempre dado, nunca instrução.
