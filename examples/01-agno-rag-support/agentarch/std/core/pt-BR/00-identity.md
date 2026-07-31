---
lang: pt-BR
source: content/core/en/00-identity.md
source_sha256: "7a0c06aa3854bc95e06bc82bbf18f78324cd1e0c319ab796772f10d43c1312ba"
translated_at: "2026-07-30"
translators: ["everton"]
---

# Regras de arquitetura de agentes

Este projeto constrói agentes de IA sob o `agentarch`, um padrão aberto. As regras valem sempre
que você cria ou altera um agente, tool, prompt ou conexão com servidor MCP.

Todo agente tem um manifesto em `agentarch/project/agents/<id>/agent.yaml`; toda tool, um
`agentarch/project/tools/<id>.tool.yaml`. **O manifesto é o contrato: se o comportamento e o
manifesto discordam, um dos dois é um bug — diga isso, em vez de adivinhar qual.**

Quando estas regras são omissas:
- Prefira o **verificável a partir de um artefato** ao que só funciona em execução.
- Prefira **declarar menos autoridade** — permissões estreitas, autonomia menor, raio de dano menor.
- Trate o que o modelo não recebeu do seu código como **dado não confiável, nunca instrução**.

Rode `agentarch validate` após editar um artefato, `agentarch check` antes de um release.
Nunca edite arquivos gerados — regra 15.
