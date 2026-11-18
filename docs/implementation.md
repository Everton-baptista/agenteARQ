# Implementação do plano AgenteARQ

Implementação local em 16/11/2026. O AgenteARQ mantém o CLI Go `agentarch` e o padrão aberto,
e acrescenta coordenação de desenvolvimento dentro do assistente que o usuário já utiliza.
O painel é local e somente leitura. Não existe um segundo runtime de modelo nem uma nova
chave de API exigida para o coordenador.

## Escopo entregue

| Frente | Implementação | Evidência disponível |
|---|---|---|
| Integridade das avaliações | Limiares lidos do plano; casos positivos; sujeito e hashes dos datasets atuais; datas futuras rejeitadas; opinião de LLM não bloqueia sozinha | Testes positivos e negativos em `internal/policy` |
| Contratos executáveis | `file_exists` consulta arquivo real dentro do projeto; evidência declarada não vira prova automaticamente | Testes de arquivo ausente, symlink, escape e remoção após leitura |
| Blueprints | Aprovação atômica por chamada, argumentos e identidade; uso único; limites cumulativos; isolamento de memória; schemas reais | 464 testes Python nas oito variantes; nove dry-runs de avaliação |
| Runtimes honestos | Remoção das cópias anunciadas como Agno; Python nativo e adaptador LangGraph efetivo | Matriz dos sete blueprints: criação, validate, check e conformance |
| Fluxo de produto | Brief, especificação, ADR, tarefas, checkpoints, dependências e retomada por worktree | Doze cenários determinísticos e testes de concorrência |
| Verificação | Perfis TypeScript web/Python API; contêiner sem rede, credenciais ou diretório Git; imagem por digest; estado inconclusivo em falhas de infraestrutura | Smoke Docker real: aprovação, alteração da fonte, timeout e remoção do contêiner |
| Revisão e PR | Evidência vinculada ao commit/branch/fonte; revisão separada; registro da URL do PR no origin correto | Testes de autor/revisor, commit modificado e URL divergente |
| Plugins | Codex, Claude Code, Gemini e contrato externo; skills contextuais, instalação/check/remoção, backup, proteção de configurações alheias | Testes de lifecycle, contratos, deriva, concorrência e symlinks |
| MCP | Cinco ferramentas permitidas, schemas estritos, revisão otimista, cancelamento e limites | Testes de protocolo, entrada inválida e cancelamento sem mutação |
| Interface | Painel React com visão geral, tarefas, qualidade e contexto; sessão local, Host/Origin e política somente leitura | Playwright: onboarding, atualização, navegação, celular, teclado, API e axe WCAG AA |
| Distribuição | Installer exige bundle Sigstore e identidade exata da release; Actions por SHA; CI reutilizável antes da release; Dependabot e análises de segurança | 13 cenários herméticos do installer; actionlint sem findings |
| Benchmark | 12 cenários × dois métodos × três repetições; runner explícito; testes HTTP independentes; evidência AgenteARQ conferida | 13 testes do harness; dry-run com 72 tentativas e zero resultado de modelo |

## Verificação executada

- `go test -race ./...` e `go vet ./...` passaram. Compatibilidade testada também com Go 1.22.12.
- Os 35 passos shell do job `examples` passaram em cópia isolada, incluindo importações Python,
  exemplos negativos, schemas, conformance, geração de contratos e atualização de conteúdo.
- Os sete blueprints foram criados e passaram `agentarch validate` e `agentarch check`.
  O nível L2 reconhece configuração e artefatos; L3 não foi concedido sem avaliação medida.
- Um cliente real do SDK MCP completou inicialização, descoberta das cinco ferramentas, leitura
  do workflow e rejeição de uma mutação inválida.
- O frontend passou no build, na navegação desktop/mobile e na auditoria automatizada de contraste
  e acessibilidade da tela avaliada. Isso não substitui uma auditoria manual com tecnologias assistivas.
- `npm audit` não encontrou vulnerabilidades. `govulncheck` não encontrou vulnerabilidades
  alcançáveis ou em pacotes importados; informou duas em partes não usadas de `x/text`.
- Smoke OCI executado com Docker 29.7.2 e Alpine identificado por digest. Asserções e reprodução
  ficam em `scripts/test-workflow-live.sh` e [workflow.md](workflow.md).

## Limites que permanecem explícitos

O benchmark com modelos reais **não foi executado**. O dry-run não permite afirmar que o AgenteARQ
é superior a outro método. [benchmark.md](benchmark.md) define o protocolo para essa medição,
com versões fixadas, custos desconhecidos como `null` e resultados separados por condição.

As integrações foram verificadas localmente no contrato, arquivos e protocolo MCP. A descoberta
e as permissões efetivas nas sessões reais de Codex, Claude e Gemini dependem da configuração
e confiança de cada host. O plugin não intercepta ferramentas externas nem autentica a identidade
de um revisor. A instalação registra limites pretendidos; não transforma instruções em sandbox.

Os blueprints usam adaptadores demonstrativos e recusam configuração de produção sem os serviços
necessários. As receitas de imagens de verificação dependem dos locks e ferramentas do produto;
a execução exige uma imagem preparada e instalada. O benchmark externo deve rodar em sandbox
próprio, pois o harness não fornece isolamento de sistema operacional.

CI, CodeQL e release foram configurados e validados estaticamente. Nenhum job remoto de GitHub,
publicação, merge ou deploy foi executado nesta implementação. Não houve chamada de modelo nem
uso de credenciais de provedor para os testes.

## Uso inicial

O binário desta sessão está em `bin/agentarch` (artefato local, ignorado pelo Git).
Para abrir o painel agora, execute `./bin/agentarch ui` na raiz deste repositório.
Para reconstruir com Go instalado:

```sh
go build -o bin/agentarch ./cmd/agentarch
./bin/agentarch workflow --help
./bin/agentarch plugin install --help
./bin/agentarch ui
```

Inicialize o workflow dentro do repositório Git do produto. Informe uma pessoa responsável,
contato e o modelo efetivamente usado antes de instalar o plugin. Para a sequência completa,
consulte [workflow.md](workflow.md), [plugins.md](plugins.md) e [verifier-images.md](verifier-images.md).
