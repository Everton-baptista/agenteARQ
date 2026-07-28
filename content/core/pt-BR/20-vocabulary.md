---
lang: pt-BR
source: content/core/en/20-vocabulary.md
source_sha256: "a6d65b7d73f1c06c2ecd7d42167f8fdaf34994baa26fc5936b8b9fadb784cf60"
translated_at: "2026-07-28"
translators: ["everton"]
---

# Vocabulário

Use estas palavras com estes significados. Precisão aqui evita classes inteiras de erro de projeto.

- **Agente** — sistema que escolhe ações para atingir um objetivo. Se não pode chamar uma tool,
  é um prompt, não um agente.
- **Manifesto** — `agent.yaml`. O contrato declarado de um agente. Fonte da verdade.
- **Tool** — capacidade com contrato tipado e permissões declaradas. Definida em um
  `.tool.yaml`, nunca só em código.
- **Effect** — o que a tool faz ao mundo: `read`, `write`, `irreversible`, `money`,
  `communication`. Determina aprovação e guardrails.
- **Autonomia** — `L0_suggest`, `L1_act_with_approval`, `L2_act_reversible`,
  `L3_act_irreversible_bounded`, `L4_autonomous`. Até onde o agente vai sem supervisão.
- **Guardrail** — verificação em um de três pontos: **input** do usuário, **output** do modelo,
  **ação** de tool. Não é sinônimo de instrução no prompt.
- **Fail mode** — o que acontece quando um guardrail não consegue decidir: `fail_closed`
  (bloqueia), `fail_warn` (permite e registra), `fail_open` (permite).
- **Conteúdo não confiável** — tudo que não foi escrito pelo seu código: texto do usuário,
  documentos recuperados, páginas web, resultados de tool, saída de outros agentes. Sempre dado,
  nunca instrução.
- **Control** — uma regra verificável, `control.ai.<tipo>.<nome>`. Tem prosa e checagem executável.
- **Pack** — conjunto versionado de controls com severidades. Dado, nunca código.
- **Perfil** — quais packs se aplicam aqui: `minimal`, `standard`, `regulated`.
- **Gate** — `agentarch check`. Bloqueia um release na severidade `blocker`.
- **Waiver** — exceção com prazo e dono. Máximo 90 dias, nunca permanente.
- **Handoff** — um agente transferindo trabalho a outro, com payload tipado, autoridade
  declarada, ponto de retorno e timeout.
- **Gatilho de revalidação** — mudança que invalida a garantia anterior: modelo, system prompt,
  corpus de RAG, provider, nova tool, autonomia elevada, guardrail desligado, novo servidor MCP.
- **Evidência** — artefato que prova que um control vale: resultado de eval, teste, hash,
  atestação. Distinta de uma declaração.
- **Shim** — arquivo de instrução gerado para assistente (`AGENTS.md`, `CLAUDE.md`, …). Saída,
  nunca entrada.
