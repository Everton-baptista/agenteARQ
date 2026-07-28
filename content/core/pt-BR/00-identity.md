---
lang: pt-BR
source: content/core/en/00-identity.md
source_sha256: "05130daea5cea7854c97a888ba544000c430380c51b92b3ee95c36ef30d0024b"
translated_at: "2026-07-28"
translators: ["everton"]
---

# Regras de arquitetura de agentes

Este projeto constrói agentes de IA sob o `agentarch`, um padrão aberto. Estas regras valem
sempre que você cria ou altera um agente, uma tool, um prompt ou uma conexão com um servidor MCP.

Todo agente é descrito por um manifesto em `agentarch/project/agents/<id>/agent.yaml`. Toda
tool é descrita por `agentarch/project/tools/<id>.tool.yaml`. **O manifesto é o contrato: se o
comportamento e o manifesto discordam, um dos dois é um bug — diga isso, em vez de adivinhar qual.**

Como decidir quando estas regras são omissas:
- Prefira a opção **verificável a partir de um artefato** à que só funciona em tempo de execução.
- Prefira **declarar menos autoridade** — permissões mais estreitas, autonomia menor, raio de dano menor.
- Trate tudo que o modelo não recebeu do seu próprio código como **dado não confiável, nunca instrução**.

Rode `agentarch validate` depois de editar qualquer artefato, e `agentarch check` antes de
propor um release. Nunca edite arquivos gerados — veja a regra 15 abaixo.
