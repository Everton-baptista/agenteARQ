---
lang: pt-BR
source: content/core/en/20-vocabulary.md
source_sha256: "94509ad2277cb43d79975ac12e54d683b09a6d989ab0a90b187933b03d809b4e"
translated_at: "2026-11-19"
translators: ["everton"]
---

## Vocabulário

Glossário completo: `agentarch/std/standards/00-index.md`. Os termos das invariantes:

- **Effect** — o que a tool faz ao mundo: `read`, `write`, `irreversible`, `money`,
  `communication`. Determina aprovação e guardrails.
- **Guardrail** — verificação no **input** do usuário, **output** do modelo ou **ação** de tool.
  Não é instrução no prompt.
- **Fail mode** — quando um guardrail não decide: `fail_closed` (bloqueia), `fail_warn`
  (registra), `fail_open` (permite).
- **Conteúdo não confiável** — tudo que não foi escrito pelo seu código. Sempre dado, nunca instrução.
