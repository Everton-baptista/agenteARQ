# Benchmark de produtos

O benchmark mede a entrega de produtos com API HTTP e uma interface web mínima em 12 cenários: seis produtos
implementados em Python e TypeScript, cada um com dois métodos e três repetições.
São **72 execuções** no conjunto completo. Isso é diferente dos cenários determinísticos
que testam o próprio workflow Go.

O harness está em `scripts/benchmark-products.py`, usa somente a biblioteca padrão
Python 3.10+ e **não chama modelos**. Um runner externo, fornecido explicitamente pelo
operador, controla o assistente. O adaptador explícito `scripts/benchmark-host-runner.py` implementa Codex; Claude e
Gemini continuam aceitos pelo protocolo externo, mas esse adaptador os recusa até
existir uma implementação revisada. O modo dry-run jamais aciona o adaptador.

## O que é medido

| Produto, em ambas as linguagens | Contrato observado |
|---|---|
| Tarefas | Criar, concluir, validar tipos, rejeitar duplicata, persistir |
| Favoritos | Validar URL, filtrar tags, excluir, persistir |
| Despesas | Valores inteiros em centavos, agregação exata, rejeitar inválidos |
| Estoque | Ajustes, rejeitar saldo negativo sem alterar estado |
| Reservas | Conflitos de intervalos, adjacência, isolamento por recurso |
| Eventos | Idempotência e preservação do primeiro evento |

Cada produto é reiniciado durante os testes para conferir durabilidade. Os casos
HTTP e requisitos completos estão em `benchmarks/products.json`. `GET /health`
retorna `200 {"ok":true}`; os endpoints de API retornam JSON, inclusive erros.
`GET /` retorna uma página HTML funcional para cadastrar um registro e visualizar
o resultado. O programa
recebe `HOST=127.0.0.1`, `PORT` e `DATA_DIR`. Dados devem ficar em `DATA_DIR`, fora
do código, para não invalidar a evidência de revisão durante os testes.

A suíte atual é **products-web-v2**, definida antes de qualquer execução com modelo.
Ela acrescenta UI funcional, labels, navegação/submissão por teclado, foco visível e
feedback. O harness verifica a superfície HTML estática; os testes do produto devem
executar a jornada e acessibilidade em navegador real headless. Em TypeScript isso
inclui `test:a11y`; em Python, integra `pytest`. Um teste vazio ou uma alegação de
que a aplicação não possui interface não atende ao contrato. A análise HTML sozinha
não prova comportamento de teclado nem acessibilidade completa.

Os braços são `executor_only` e `agentarch`. Cada par recebe a mesma linguagem,
contrato, host, versão, modelo e limite de tempo do runner. A ordem é alternada
por sorteio reproduzível, com seed registrado. São sessões e diretórios novos,
sem reaproveitar a solução do outro braço. Os testes são públicos: não se trata
de uma avaliação oculta ou resistente a treinamento sobre os casos.

O harness registra, por execução:

- `passed`, `failed` ou `inconclusive`, além de cada check HTTP e sua duração;
- duração medida do runner e da execução completa, em milissegundos;
- custo informado pelo runner, com `cost_source=reported_by_runner`, ou `null`;
- hashes do conjunto de cenários, harness e binário AgenteARQ;
- no braço tratado, exportação e snapshots atuais das evidências de workflow.

Custo ausente nunca vira zero. A soma dos custos conhecidos vem acompanhada da
quantidade de execuções sem informação; não é o custo total se houver desconhecidos.
Não são produzidos scores subjetivos, estimativas de custo ou alegações de ganho.

## Validar a matriz sem executar hosts

Da raiz do repositório:

```sh
python3 scripts/benchmark-products.py --dry-run > /tmp/products-matrix.json
python3 -m unittest discover -s scripts/tests -p 'test_benchmark*.py' -v
```

Sem `--execute`, o comportamento padrão também é dry-run. Esse modo valida o
contrato e a matriz, deixa `results=[]` e `summary=null`. Mesmo com uma configuração
de runner, não inicia processos do runner, candidatos, hosts ou modelos.

## Contrato do runner externo

O operador escolhe uma implementação confiável de runner e fornece um JSON:

```json
{
  "schema_version": "1.0",
  "host": "codex",
  "host_version": "VERSAO_EXATA_DO_HOST",
  "model_id": "IDENTIFICADOR_EXATO_DO_MODELO",
  "command": ["python3", "/caminho/absoluto/meu_runner.py"],
  "agentarch_bin": "/caminho/absoluto/agentarch",
  "runner_timeout_seconds": 1200
}
```

Esses valores são exemplos para substituir. O modelo e a versão são declarados
pelo operador/runner; o harness compara igualdade entre pedido e resposta, mas não
consulta provedores para provar que um nome corresponde a um snapshot imutável.
Não coloque credenciais no JSON ou nos argumentos. Registre também a revisão do
adaptador e a configuração de recursos/sandbox ao publicar uma medição.

O comando recebe um único objeto JSON pelo **stdin** e deve imprimir somente um
objeto JSON no **stdout**. Ele executa com o diretório da tentativa como cwd. O
pedido inclui `schema_version`, `run` (id, cenário, linguagem, braço, repetição),
`host`, `host_version`, `model_id`, `seed`, `working_directory`,
`runner_timeout_seconds`, `common_requirements`, `scenario` (incluindo checks),
`agentarch_bin` e `condition_requirements`.

O runner controla a sessão do assistente e prepara o produto, incluindo dependências
e build; o harness não instala pacotes. Seu orçamento de tempo inclui construção,
verificação e revisão quando exigidas pelo método. A resposta é:

```json
{
  "schema_version": "1.0",
  "status": "completed",
  "host": "codex",
  "host_version": "VERSAO_EXATA_DO_HOST",
  "model_id": "IDENTIFICADOR_EXATO_DO_MODELO",
  "candidate_dir": "candidate",
  "start_command": ["python3", "server.py"],
  "cost_usd": null,
  "agentarch_evidence": {
    "export": "agentarch/project/evidence/latest.json"
  }
}
```

`candidate_dir` é relativo ao diretório da tentativa e não pode escapar dele por
`..` ou symlink. `start_command` é um vetor de argumentos, executado no candidato
com `shell=False`. Para TypeScript, pode apontar para o JavaScript já compilado,
por exemplo `['node', 'dist/server.js']`. O custo inclui todas as chamadas e revisão
da tentativa, quando o runner consegue medi-lo. Para falha de construção, responda
`status=failed`; para infraestrutura ou evidência indisponível use `status=inconclusive`,
mantendo identidade de host/modelo e `cost_usd`; candidato e comando
não são necessários. Campos desconhecidos e respostas malformadas são rejeitados.

No braço `executor_only`, omita `agentarch_evidence` e use uma sessão/configuração
do host que não carregue skills, MCP ou instruções AgenteARQ. O harness recusa um
workflow AgenteARQ encontrado no candidato desse braço. Isso não prova ausência de
instruções globais no host: a separação das sessões é responsabilidade do runner e
deve ser auditável na metodologia publicada.

No braço `agentarch`, o runner deve instalar o plugin do host no candidato,
inicializar o workflow com o perfil do cenário e declarar o executor. Deve concluir
as etapas, executar `agentarch workflow verify` com verificador OCI e obter revisão
independente com `agentarch workflow review`. A exportação acima é escrita pela CLI.
**Não escreva uma aprovação fictícia para completar a tentativa.** Se não houver
revisor ou verificador disponível, a tentativa não tem evidência suficiente.

O harness executa por conta própria `workflow status --format json` e
`plugin check --host ... --format json` antes e depois dos testes. Exige:

- workflow `pr_ready`, sem findings, executor declarado e plugin instalado íntegro;
- exportação igual aos registros atuais de verificação e revisão da CLI;
- verificação aprovada, checks presentes e aprovados, isolamento OCI registrado;
- revisão aprovada por pessoa/agente identificado, diferente do autor;
- fingerprints de código, HEAD, worktree e perfil iguais ao snapshot atual.

Exportação ausente, alterada ou stale resulta em `inconclusive`, nunca em sucesso.
O harness salva snapshots e cópias da exportação no diretório da tentativa. A
identidade declarada do revisor não demonstra por si só independência cognitiva;
essa limitação permanece explícita.

## Adaptador real do Codex

O adaptador requer Python 3.11+ (`tomllib`) e uma CLI Codex autenticada pelo operador.
Não cria credenciais nem altera `HOME`, `CODEX_HOME`, arquivos de autenticação,
configuração global ou regras de segurança. Foi implementado para as opções locais
da CLI 0.160.1 e segue a [execução não interativa oficial](https://developers.openai.com/codex/noninteractive/)
e a [referência de configuração](https://developers.openai.com/codex/config-reference/).

Um exemplo de vetor `command` para o harness, substituindo caminhos e digests:

```json
[
  "python3", "/repo/scripts/benchmark-host-runner.py",
  "--host-binary", "/caminho/codex",
  "--owner", "NOME_DA_PESSOA_RESPONSAVEL",
  "--contact", "CONTATO_FORNECIDO",
  "--verifier-image-python", "imagem-python@sha256:DIGEST_REAL",
  "--verifier-image-typescript", "imagem-typescript@sha256:DIGEST_REAL"
]
```

O binário AgenteARQ e o modelo vêm do pedido do harness. Ambos os braços usam o
mesmo modelo e `model_reasoning_effort=medium`, até três tentativas, com no máximo
1200 segundos totais. `--implementation-seconds` (480 por padrão) e
`--review-seconds` (180) limitam cada sessão dentro desse orçamento cumulativo.
Uma sessão não retoma a memória de outra. Cada revisão ocorre em nova sessão,
também no braço controle. O limite de 240 mil tokens usa **entrada não cacheada +
saída observadas**, e impede uma próxima sessão. Entrada total, entrada cacheada e
saída são sempre reportadas separadamente. Se a CLI não informar cache, conta-se a
entrada inteira conservadoramente; ausência de uso essencial bloqueia a tentativa.
A CLI informa uso ao terminar o turno, portanto isso não garante um limite financeiro
antes de uma chamada ao provedor. Essa definição foi fixada antes dos trials após
um preflight real de 21 segundos reportar 107.911 tokens de entrada, dos quais
89.728 cacheados, e 273 de saída. Preflight não é resultado de produto.

O implementador usa `workspace-write`, o revisor usa `read-only`, com aprovação
`never` (falhas não geram escalonamento), rede de shell desativada, sem ampliar
raízes de escrita para `/tmp`. O adaptador usa `--ignore-user-config`,
`--ephemeral` e `--no-daemon`, preserva as regras e jamais usa flags de bypass.
Essas opções separam [sandbox e aprovação](https://developers.openai.com/codex/security/).
A autenticação e o transporte do provedor permanecem a cargo do Codex.

Skills globais genéricas permanecem iguais nos dois braços. A presença detectada
de uma skill global AgenteARQ interrompe o runner para evitar contaminação. Plugins
globais descobertos nos diretórios de cache são desabilitados apenas por overrides
na invocação; `--disable-plugin ID` permite completar a lista. O inventário de
caminhos/hashes e IDs desabilitados vai para a evidência. Esse inventário não prova
a ausência de toda instrução externa gerenciada pelo host.

No tratado, o runner instala o plugin e lê somente sua entrada MCP gerenciada. Para
compatibilidade com a CLI 0.160.1, passa explicitamente command/args/allowlist via
configuração da invocação. Não habilita hooks alheios. O implementador recebe apenas
os três tools de leitura e `record_checkpoint`; o revisor recebe só os de leitura.
O processo coordenador confiável aplica **os mesmos comandos do perfil e a mesma
imagem OCI nos dois braços**. No controle usa `agentarch verify-profile`, instrumento
de medição sem inicializar workflow, instalar skills/MCP ou escrever artefatos no
candidato; no tratado usa `workflow verify`, acrescentando os gates do wrapper.
Os dois revisores recebem resultados determinísticos reais. O coordenador só registra
`workflow review` depois de uma resposta real da sessão independente, com fingerprint
inalterado. A implementação deve executar uma chamada MCP observada com sucesso.

No coding host, `completed` significa que código e testes estão prontos para o
coordenador, **sem afirmar que passaram**. Falta de toolchain, navegador ou loopback
no host não torna essa entrega `blocked`: os checks de servidor/navegador serão
executados em OCI. O modelo pode executar checks locais de arquivos/CPU disponíveis,
mas não deve ampliar permissões nem iniciar servidores no host. Checkpoints do
tratado usam exclusivamente MCP `record_checkpoint`, com `expected_revision` obtida
por `get_workflow`; não há fallback de escrita direta em `.git` pela CLI do host.
Nos tasks TypeScript os IDs são `lint`, `typecheck`, `test`, `build`, `e2e`,
`accessibility`, `security`; `test:a11y` é o comando npm, não um ID de check.

No perfil Python, o comando de segurança é
`python -m bandit -r . -q -x './.venv/*,./tests/*'`. As exclusões abrangem somente
`.venv/` e `tests/` na raiz, incluindo suas subpastas; os testes pytest devem ficar
nesse `tests/`. `src/tests/`, `tests_support/` e scripts de skills gerenciadas
continuam na análise. O check `B101` permanece ativo no código da aplicação;
não se aplica `--skip B101` global. Os padrões explícitos com `./` são necessários
para coincidir com os caminhos descobertos pelo Bandit 1.9.4 ao analisar `.`.
A regressão `TestPythonBanditScopeOCI` executa o próprio comando do perfil na
imagem OCI e comprova ambas as fronteiras. Resultados anteriores preservam o
comando e os hashes com que foram produzidos; a correção não reclassifica esses
resultados nem permite misturar execuções de protocolos diferentes.

Para que o reparo receba erros de compilação/teste úteis, o operador pode acrescentar
à configuração explícita do runner:

```json
["--diagnostics-reason", "Synthetic public benchmark fixtures; compiler and test failures required for repair."]
```

O runner transmite essa opção às duas CLIs de verificação. O padrão continua
hash-only; com a opção, o verificador retorna um prefixo redigido de até 16 KiB,
contagem de bytes, indicador de truncamento e hash do stream completo. Os diagnósticos
entram como dados não confiáveis no feedback de reparo. Capturam somente a saída de
compiladores/testes das fixtures, não conversas do modelo. Redação de padrões não
é garantia de remoção de todo segredo/PII; os dados precisam ser sintéticos públicos.

O executor declarado do produto é **benchmark-codex-runner**, um contrato separado
materializado pelo adaptador. Ele descreve controles reais dessa fronteira: entrada
limitada/estruturada com triagem básica, prompt developer constante versionado e
hashado, dados no bloco de usuário, JSON final estrito e bloqueio de padrões de
credencial antes de persistir, argv fixo, deadline, proteção dos artefatos gerenciados
e revisão distinta. O manifest nativo **agentarch-codex permanece inalterado** com
suas limitações declaradas. O gate aplicado ao wrapper não certifica toda ação,
saída interna ou ferramenta arbitrária do Codex. Não se aplica waiver para esconder
limitações do plugin.

As imagens verificadoras devem estar preparadas, fixadas por digest e disponíveis
localmente. Python precisa das ferramentas do perfil e Playwright/Chromium para os
testes UI; TypeScript precisa Node, TypeScript, ferramentas de testes e navegador.
O servidor deve iniciar no host com Python stdlib ou Node built-ins; por exemplo,
`python3 server.py`, `node --experimental-strip-types server.ts` com sintaxe suportada,
ou JavaScript de runtime versionado. Não dependa de pacotes disponíveis somente na
imagem nem de `dist` gerado por outro check: cada comando OCI recebe um snapshot
limpo e independente, então scripts de teste preparam seus próprios artefatos.
O runner não baixa dependências nem executa código gerado fora da sessão confinada
para contornar um verificador ausente. O harness HTTP continua tendo sua própria
limitação de isolamento, descrita abaixo.

Os shell tools do host recebem `TMPDIR` apontando para `candidate/.benchmark-tmp`,
criado com modo `0700` e ignorado pelo Git. Isso não altera `HOME`, `CODEX_HOME`,
o ambiente do coordenador nem os diretórios permitidos pelo sandbox; a sessão de
revisão continua somente leitura. Temporários locais do pytest usam essa pasta,
sem gravar caminhos absolutos do host na configuração versionada. O runner rejeita
redirecionamento da pasta, temporários adicionados à força e exceções de ignore que
exponham arquivos. O `product.yaml` criado pelo coordenador também é protegido por
hash: perfil e executor não podem ser enfraquecidos pelo modelo durante reparos.

Violações determinísticas detectadas durante a cópia do snapshot, como um symlink
absoluto/que escapa da árvore ou um tipo de arquivo proibido, recebem `failed`
antes de executar o container; erros de I/O nessa etapa continuam `inconclusive`.
Erros anteriores à medição, como a impossibilidade de obter o fingerprint, ainda
retornam erro da CLI. A proteção contra symlinks permanece ativa. Mudanças no runner
ou no verificador exigem uma nova coorte, preservando os resultados já produzidos.

`host-runner/execution.json` e os arquivos `*-metadata.json` registram IDs de sessão,
uso informado pelo host, chamadas MCP, duração e hashes. Respostas estruturadas
validadas e pareceres ficam ao lado; eventos JSONL brutos não são retidos. Credenciais
com padrões conhecidos bloqueiam a saída. A triagem não detecta todo segredo possível.
Custo monetário continua `null` quando a CLI fornece apenas tokens.

Um identificador fixo do catálogo do host, como o selecionado no experimento, é
registrado literalmente. Ele não demonstra um snapshot imutável do provedor; publique
essa limitação junto dos resultados. Um preflight de conexão ou os testes unitários
do adaptador não são resultados dos 72 casos de produto.

## Executar uma medição autorizada

```sh
python3 scripts/benchmark-products.py \
  --execute \
  --runner-config /caminho/runner.json \
  --output-dir /tmp/products-benchmark-001
```

O diretório de saída deve ser novo; resultados anteriores não são sobrescritos.
`report.json` é atualizado atomicamente após cada tentativa. Cada subdiretório guarda
o pedido, resposta validada, resultados e evidências. O harness não retém stderr
arbitrário do runner nem logs do servidor, que podem conter dados sensíveis.

Para um piloto, acrescente `--scenario python-tasks`: continua comparando ambos os
braços com três repetições, informa seis tentativas e `complete_suite=false`. Use
`--seed N` para repetir a ordem. A saída JSON também vai para stdout; progresso vai
para stderr. Exit code: `0` dry-run válido ou todas as tentativas aprovadas, `1`
alguma tentativa falhou/ficou inconclusiva, `2` configuração inválida.

A primeira tentativa `inconclusive` interrompe a coorte **antes de iniciar outra**.
Erros nativos do host são reduzidos a categorias enumeradas (`quota_exhausted`,
`rate_limited`, `authentication_required`, `network_unavailable`,
`provider_unavailable`, `other`), código conhecido e hash; a mensagem bruta não é
armazenada. Mensagens do modelo ou de tools não são tratadas como erro de quota do
host. Uso conhecido da sessão com falha é preservado; ausência de uso fica explícita.
Falhas objetivas do produto permanecem `failed` e não acionam essa parada de
infraestrutura. Após corrigir a causa, faça uma nova coorte com seus próprios hashes.

`matrix_complete` indica os 72 casos planejados; `execution_complete` indica que
todas as tentativas selecionadas terminaram. `complete_suite` só é verdadeiro se
as 72 foram executadas, independentemente de aprovação ou falha. `completed_runs`,
`remaining_runs`, `aborted`, `abort_reason` e `not_run` tornam a execução parcial
explícita. Tentativas não iniciadas nunca entram como falhas ou aprovações. Dry-run
pode ter matriz completa, mas não execução completa. Uma interrupção abrupta também
pode deixar o relatório parcial; compare `results` com `planned_runs`.

A execução atualmente requer POSIX. O harness limita tempo e saída dos processos,
usa somente HTTP loopback sem proxy/redirect e encerra os grupos de processos que
inicia. **Isso não é isolamento de sistema operacional.** Rode a medição em ambiente
dedicado e isolado: o runner é código explicitamente autorizado e o candidato também
será executado. O processo candidato recebe ambiente reduzido sem chaves herdadas;
o runner pode precisar do ambiente de autenticação do host. O harness não impede
que código malicioso acesse arquivos/rede disponíveis no ambiente. A evidência OCI
do workflow cobre seus checks; não transforma a execução HTTP do harness em sandbox.

## Limites de interpretação

Uma aprovação significa que os contratos de API/superfície HTML passaram e, no braço tratado, os
registros atuais exigidos foram confirmados. Os checks de UI são limitados e não estabelecem UX ou acessibilidade completa. Não mede segurança
completa, escala, manutenção futura ou a capacidade de construir qualquer software.
Não comprova que toda ação do assistente passou pelo MCP, nem impede um runner
malicioso de forjar sua parte do experimento. Os testes são pequenos e sintéticos.

Três repetições permitem observar variação, mas não sustentam por si só uma conclusão
causal geral. Publique resultados individuais, falhas/inconclusivos, recursos,
versões, orçamento igual, política de revisão e custos desconhecidos. Compare
separadamente cada produto e linguagem antes de agregar; não descarte tentativas
malsucedidas ou substitua `inconclusive` por aprovação. O repositório fornece o
instrumento e seus testes, sem resultados fabricados de execuções com modelos.
