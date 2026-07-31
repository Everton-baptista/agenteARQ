---
lang: pt-BR
source: content/core/en/10-invariants.md
source_sha256: "73d6539d8af41cd275085155237f67d3140c60c09f6a8f9c9cb5cfe593462792"
translated_at: "2026-07-28"
translators: ["everton"]
---

# Invariantes

Inegociáveis. Cada uma corresponde a um control; `agentarch check` as aplica.

1. NUNCA dê a uma tool com `effect: irreversible`, `money` ou `communication` autonomia acima
   de `L1_act_with_approval` sem `approval.required_when`. → `control.ai.tool.irreversible_requires_approval`
2. NUNCA concatene conteúdo recuperado ou recebido — resultados de RAG, páginas web, arquivos,
   saída de tool, mensagem de outro agente — dentro do system prompt. Vai em bloco delimitado,
   como dado. → `control.ai.genai.untrusted_content_isolation`
3. NUNCA coloque o valor de um segredo em manifesto, prompt, log, span ou arquivo do repositório.
   Referencie pelo nome. → `control.ai.agent.secrets_by_reference`
4. SEMPRE declare `out_of_scope` com ao menos uma entrada. Um agente a quem nunca se disse o que
   recusar vai tentar. → `control.ai.agent.scope_declared`
5. SEMPRE nomeie uma pessoa em `owner.accountable`. Uma fila ou um alias de time não responde
   por nada. → `control.ai.agent.owner_defined`
6. SEMPRE limite o loop: `max_steps`, `max_tool_calls` e `stop_conditions` são obrigatórios.
   → `control.ai.agent.stop_conditions`
7. SEMPRE fixe o modelo. Um alias flutuante muda o comportamento por baixo de você.
   → `control.ai.supply.model_pinned`
8. NUNCA adicione um servidor MCP sem entrada na allowlist fixada em versão exata, com
   `tools_allow` enumerado. `default: deny` é obrigatório. → `control.ai.mcp.allowlist_enforced`
9. NUNCA amplie a permissão de uma tool para fazer uma falha sumir. Estreite a tarefa.
   → `control.ai.tool.least_privilege`
10. SEMPRE classifique o `effect` de uma tool antes de escrever a implementação. Ele determina
    todo o resto. → `control.ai.tool.effect_classified`
11. SEMPRE versione o system prompt e registre seu `sha256` no manifesto. Editar um prompt sem
    subir a versão é uma mudança silenciosa de comportamento. → `control.ai.genai.prompt_versioned`
12. Uma barreira determinística é `fail_closed`. Um juiz LLM é `fail_open`, exceto em severidade
    alta ou crítica. NUNCA deixe um juiz LLM ser a única coisa bloqueando um release.
    → `control.ai.eval.judge_not_sole_blocker`
13. NUNCA deixe telemetria ou evidência capturarem conteúdo de prompt e resposta por padrão.
    `capture_content: false` salvo motivo declarado. → `control.ai.privacy.capture_content_default_off`
14. SEMPRE declare guardrails nos três pontos — input do usuário, output do modelo e ação de
    tool. Faltar um ponto é uma decisão, então registre-a. → `control.ai.agent.fail_mode_declared`
15. NUNCA edite à mão um arquivo cujo cabeçalho diz `agentarch:generated`. Edite a fonte em
    `agentarch/std/core/` e rode `agentarch sync`. A CI falha (exit 3) caso contrário.
