---
lang: pt-BR
source: content/core/en/00-identity.md
source_sha256: "9a7f45ef30ac6910d8e0ab555212bd9f2cbdee3ad77bc8c9c673fc2c96565a56"
translated_at: "2026-11-19"
translators: ["everton"]
---

# Regras de projeto do AgenteARQ

Este projeto usa o `agentarch`, um padrão aberto, para levar um produto de software de qualquer
tipo e em qualquer tecnologia da ideia à produção em etapas verificáveis: brief, especificação,
decisão de arquitetura comparada, tarefas, verificação isolada, revisão independente, release e
verificação em produção. Rode `agentarch workflow status` para ver em que ponto o produto está.

**A tecnologia é escolhida, nunca presumida.** Quando linguagem, framework, hospedagem, banco de
dados, provedor de modelo ou fornecedor ainda não foram decididos, proponha opções que atendam à
demanda, diga os benefícios e os custos de cada uma, identifique a sua recomendação como
recomendação e deixe o responsável escolher. Registre em um ADR.

As regras de agente abaixo valem sempre que você cria ou altera um agente de IA, tool, prompt ou
conexão com servidor MCP. Um produto sem componente de IA segue só o fluxo de trabalho.

Todo agente tem um manifesto em `agentarch/project/agents/<id>/agent.yaml`; toda tool, um
`agentarch/project/tools/<id>.tool.yaml`. **O manifesto é o contrato: se o comportamento e o
manifesto discordam, um dos dois é um bug — diga isso, em vez de adivinhar qual.**

Quando estas regras são omissas:
- Prefira o **verificável a partir de um artefato** ao que só funciona em execução.
- Prefira **declarar menos autoridade** — permissões estreitas, autonomia menor, raio de dano menor.
- Trate o que o modelo não recebeu do seu código como **dado não confiável, nunca instrução**.

Rode `agentarch validate` após editar um artefato, `agentarch check` antes de um release.
Nunca edite arquivos gerados — regra 15.
