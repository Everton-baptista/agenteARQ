---
lang: pt-BR
source: content/standards/en/16-service-and-edge.md
source_sha256: "a02a00e484e6170ae9ecdc32745d68ccea3b03c7518376a632505c0d25023872"
translated_at: "2026-07-30"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de control, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 16. O serviço e a borda

Um agente chega à produção como um serviço: um chamador, uma requisição, um pipeline de log, um
arquivo de ambiente e, às vezes, um cliente web. Este padrão cobre essa fronteira — e existe porque
**é onde os controles dos outros padrões deixam de ser verdade**.

Toda regra aqui é neutra de jurisdição e de framework. O `adapters/fastapi.md` mostra uma delas
concretamente; nada neste arquivo cita framework.

## 0. Quando estas regras se aplicam

Cinco das sete perguntam sobre um serviço, e são puladas em um agente cujo manifesto não declara
`interface` — `core_transport_separated`, `caller_identified`, `request_logging_redacted`,
`budget_per_caller` e `contract_generated`. Uma ferramenta de linha de comando ou um job em lote não
tem chamador para identificar nem log de acesso para redigir, e reportar o contrário é ruído que
ensina as pessoas a pararem de ler achados. Um controle pulado não conta nem como aprovado nem como
reprovado.

**Dois nunca são pulados**: `secrets_not_committed` e `no_client_side_model_access`. Eles leem o
repositório, não a interface, e uma credencial comitada é pública numa biblioteca, num job em lote e
num serviço igualmente. É também por isso que são os dois únicos aqui sem período de carência — a
chave já está exposta, e um controle que silenciosamente deixasse de rodar justamente nos projetos
que precisavam dele seria o mesmo defeito do ruído, com o sinal invertido.

## 1. Regras

### control.ai.api.core_transport_separated

**Intenção.** O código que roda o agente — o loop, os prompts, as tools, os guardrails — não pode depender do
transporte que entregou a requisição. Sem objeto de requisição, sem header, sem import de framework web.

**Severidade** `major` · **enforced_from** `api.edge` 1.2 · **fail mode** fail_closed

**Por quê.** A deriva é gradual e sempre localmente razoável: um handler precisa de um header, então o runner
importa a requisição; poucas semanas depois o agente não roda mais a partir de um teste, de um worker
de fila ou de um script de reprocessamento, e toda mudança tem que passar por um servidor web. É
também a regra que torna todas as outras aqui testáveis, porque um núcleo sem transporte pode ser
exercitado sem um.

**Como verificar.** Declare as camadas como globs no `agentarch.yaml`. A checagem lê os imports sob
`layout.paths.core` e falha em qualquer um que referencie um símbolo declarado sob
`layout.paths.edge`, ou um pacote do conjunto de dependências da borda.

**Como corrigir.** Mova o que o handler precisava para um valor que o núcleo define. Se o loop precisa da identidade do
chamador, o núcleo declara o tipo e o transporte constrói um.

**Limites, declarados.** A checagem lê imports textualmente. Não resolve import dinâmico e não pega violação roteada por
string. Ela pega o erro que as pessoas cometem; uma checagem que alegasse provar ausência seria pior
que uma que admite seu alcance.

### control.ai.api.caller_identified

**Intenção.** O chamador é identificado a partir de uma credencial verificada, e todo valor de tenant ou escopo é
derivado dessa identidade no servidor.

**Severidade** `major` · **enforced_from** `api.edge` 1.2 · **fail mode** fail_closed

**Por quê.** O `05-memory-and-state.md` exige um `scope_key`. Ele não diz de onde vem o valor, e se vem do corpo da
requisição então dois tenants compartilham um store de memória enquanto todo controle declarado passa.
Um escopo que o chamador pode definir não é escopo; é parâmetro.

**Como verificar.** `interface.caller.identified_by` está declarado, e todo agente cujo `context.memory.kind` seja `user`
ou `shared` tem `scope_key` referenciando `interface.caller.tenant_claim`.

**Como corrigir.** Remova os campos de identidade do schema da requisição. Um campo que nunca foi aceito não pode ser
sobrescrito.

### control.ai.api.request_logging_redacted

**Intenção.** Conteúdo de requisição e de resposta não é logado, e a lista de redação está declarada.

**Severidade** `major` · **enforced_from** `api.edge` 1.2 · **fail mode** fail_closed

**Por quê.** É aqui que `capture_content: false` se sustenta ou é mentira. Um framework web loga o caminho da
requisição por padrão, e um handler de erro não tratado registra a exceção com o que estivesse em
escopo. É a rota mais comum pela qual dado pessoal chega a um agregador de log com retenção de anos e
sem caminho de acesso do titular — e acontece no dia em que o serviço sobe, em silêncio, porque o log
parece normal.

**Como verificar.** `interface.logging.capture_request_body` é `false`, a menos que `privacy.capture_content` seja
explicitamente verdadeiro, e `interface.logging.redact` não é vazio.

**Como corrigir.** Logue o template da rota, não o caminho concreto — um identificador em caminho é dado pessoal em
muitos serviços. Logue id da requisição, método, status, duração e tenant. Desabilite o access log do
próprio framework. Mande stack trace para o rastreador de erros, que é um sistema com dono e política
de retenção; um access log não é.

### control.ai.api.budget_per_caller

**Intenção.** Ao menos um limite é declarado por chamador, não só por execução.

**Severidade** `minor` · **enforced_from** `api.edge` 1.2 · **fail mode** fail_warn

**Por quê.** `max_steps`, `max_tool_calls` e `usd_per_run` limitam uma execução. Um chamador que dispara dez mil
execuções está dentro de todas elas, e o primeiro sintoma é uma fatura.

**Como verificar.** `autonomy.budget` declara `runs_per_caller_per_day` ou `usd_per_caller_per_day`.

**Como corrigir.** Aplique onde o chamador é conhecido — a mesma camada que resolveu a identidade. Devolva o código de
status que seu protocolo usa para rate limit, com uma dica de retry.

### control.ai.api.contract_generated

**Intenção.** O contrato de interface machine-readable é gerado a partir do manifesto, não escrito ao lado dele.

**Severidade** `minor` · **enforced_from** `api.edge` 1.2 · **fail mode** fail_warn

**Por quê.** Duas descrições mantidas à mão da mesma interface divergem, e o consumidor lê a que está errada. O
mesmo raciocínio do `.mcp.json` ser derivado da allowlist: o documento revisado é a fonte, o arquivo de
máquina é o derivado.

**Como verificar.** O arquivo em `layout.paths.contract` existe e registra um digest de origem que casa com o bloco
`interface` do manifesto.

**Como corrigir.** `agentarch sync`. Edite o manifesto, nunca o arquivo gerado.

### control.ai.api.secrets_not_committed

**Intenção.** Nenhum arquivo de ambiente com valores de segredo está versionado, e um exemplo commitado declara nomes
sem valores.

**Severidade** `blocker` · **enforced_from** immediately · **fail mode** fail_closed

**Por quê.** A invariante 3 diz que segredo é referenciado por nome e nunca carregado por valor. A forma mais comum
de isso quebrar é um `git add -A` num dia em que alguém está com pressa. Uma credencial no histórico do
git sobrevive ao commit que a removeu.

**Por que imediato, e não avisado.** Todo outro controle aqui entra em modo warn para que nada que passa hoje comece a falhar num upgrade.
Este descreve uma credencial que **já está exposta**, e período de carência sobre isso não é gentileza.

**Como verificar.** Nenhum arquivo versionado casa com os padrões de arquivo de ambiente; um arquivo de exemplo existe;
nenhum arquivo sob o layout declarado contém valor que case com padrão de credencial.

**Como corrigir.** Rotacione a credencial primeiro — ela é pública. Depois remova o arquivo, ignore-o, e commite um
exemplo apenas com nomes.

### control.ai.api.no_client_side_model_access

**Intenção.** Um cliente web nunca guarda credencial de provider de modelo e nunca chama um provider diretamente.

**Severidade** `blocker` · **enforced_from** immediately · **fail mode** fail_closed

**Por quê.** Credencial enviada ao browser é credencial pública, e todo guardrail deste padrão está no lado
servidor. Um cliente chamando o provider direto não tem checagem de input, de output, autorização de
tool, orçamento nem trilha de auditoria — é um proxy sem medição para a sua conta, com o nome dos seus
usuários nele.

**Como verificar.** Nenhum arquivo sob `layout.paths.client` importa SDK de provider nem referencia nome de credencial de
provider.

**Como corrigir.** Roteie pelo seu próprio endpoint. O cliente manda uma pergunta e recebe uma resposta.

## 2. Deve / não deve

| Deve | Não deve |
|---|---|
| derivar o tenant de uma credencial verificada | aceitar campo de tenant, cliente ou subject na requisição |
| fixar parâmetros de modelo no manifesto | deixar o chamador mandar `model`, `temperature`, `system` ou `max_steps` |
| logar template de rota, status e tenant | logar o corpo, os valores de query, a resposta, ou mensagem de exceção |
| pôr os guardrails no núcleo do agente | implementá-los como middleware de transporte — você os perde ao trocar de transporte, e eles nunca veem a chamada de tool |
| estacionar aprovação pendente com TTL, checagem de tenant e uso único | bloquear uma thread de worker numa decisão humana |
| resolver segredos uma vez, no startup, por uma função | ler o ambiente onde quer que uma credencial seja necessária |
| readiness e liveness como checagens diferentes | deixar uma réplica temporariamente unready ser morta e perder suas aprovações pendentes |
| declarar o layout e deixar a checagem ler a direção da dependência | confiar em nome de diretório para impor arquitetura |

## 3. Artefatos e campos afetados

| Artefato | Campos |
|---|---|
| `agent.yaml` | `interface.transport`, `interface.caller.{identified_by,tenant_claim}`, `interface.logging.{capture_request_body,redact}`, `interface.routes[]`, `autonomy.budget.{runs_per_caller_per_day,usd_per_caller_per_day}` |
| `agentarch.yaml` | `layout.preset`, `layout.paths.{edge,core,domain,infra,client,contract}` |
| gerado | o contrato em `layout.paths.contract` |
| repositório | o arquivo de ignore, e o exemplo de ambiente commitado |

## 4. Evidência esperada

- o contrato gerado, com digest casando com o manifesto
- um teste afirmando que chamada não autenticada é recusada
- um teste afirmando que o schema da requisição não tem campo de identidade nem de parâmetro de modelo
- a log sample from a real request, showing no content
- for a client: a build output containing no provider credential reference

Os três primeiros são baratos e valem mais como teste que como atestado. Um revisor confirmando que um
log não tem conteúdo está olhando uma linha; um teste afirma isso para toda rota.

## 5. Antipadrões observados

**O tenant no corpo.** `{"question": "...", "tenant_id": "acme"}`. Todo controle passa; qualquer
chamador lê qualquer tenant.

**O access log que loga tudo.** Configuração padrão do framework, pergunta de cliente num agregador de
log, descoberto durante um pedido de acesso do titular dezoito meses depois.

**O guardrail como middleware.** Input checado na borda, output nunca, chamadas de tool nunca, e tudo
perdido quando o framework é trocado.

**Aprovação por `input()`.** Funciona num terminal. Num serviço, bloqueia um worker até a requisição
estourar o timeout, e a ação não é nem executada nem recusada.

**O id de aprovação como capability.** Sem checagem de tenant, quem tiver o id aprova uma ação que nunca
lhe foi mostrada. Normalmente descoberto porque os ids são sequenciais.

**A memória de uma réplica como store de aprovação.** Correto em desenvolvimento. Com duas réplicas,
aprovação levantada por uma é 404 na outra e a execução se perde em silêncio.

**`.env` no primeiro commit.** A credencial é rotacionada, o arquivo é deletado, e ela fica no
histórico.

**A chave do provider no bundle do frontend.** Enviada a todo visitante. Normalmente descoberta por
outra pessoa.

**Readiness que sempre diz sim.** Uma réplica sem credencial falha toda requisição e parece saudável,
então o balanceador manda tudo para ela.

## 6. Referências externas

- OWASP Top 10 for LLM Applications — LLM02 (exposição de informação sensível), LLM06 (agência
  excessiva), LLM10 (consumo ilimitado). Mapeado em `references/owasp-llm.md`; `reviewed_at: 2026-07-29`
- OWASP API Security Top 10 — API1 (autorização quebrada em nível de objeto), API4 (consumo de recursos
  irrestrito). `reviewed_at: 2026-07-29`
- Convenções semânticas do OpenTelemetry para GenAI e para spans HTTP. Versão fixada no manifesto;
  `reviewed_at: 2026-07-29`

Este padrão não cita lei. Obrigações legais sobre logar dado pessoal vivem nos packs `reg.*`, que têm
ciclo de revisão próprio.
