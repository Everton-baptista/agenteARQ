---
lang: pt-BR
source: content/standards/en/17-design-quality.md
source_sha256: "2d8180d14b4a30b3d97a2212c3a07ce42e8b4f25b967692c5462fb1ad80dc802"
translated_at: "2026-11-23"
translators: ["everton"]
---

> **Tradução.** O inglês é normativo. Se esta tradução e a fonte discordarem, a fonte está
> certa — e `agentarch validate` reporta a divergência como `AA-I18N-016` assim que o original
> muda. Ids de check, nomes de campo e nomes de arquivo permanecem em inglês em toda língua,
> para que mensagens de erro e buscas continuem interoperáveis entre times.

# 17. Qualidade de design

Propósito: interfaces que parecem intencionais, pertencem ao seu produto e são acessíveis, em vez
do visual genérico que um modelo produz quando ninguém lhe dá uma direção.
Versão: 0.1 · Status: draft · Escopo: qualquer produto com interface de usuário (web, mobile,
desktop), `design/brief.md`, `design/tokens.json`.

Um modelo a quem se pede "uma interface moderna" amostra o centro do que já viu: a mesma fonte, o
mesmo gradiente, os mesmos três cards. A causa é convergência distribucional, não falta de gosto,
então pedir gosto num prompt não resolve. Uma lista de visuais proibidos também não resolve:
proíba um default e o modelo passa para o próximo mais provável. O que funciona é uma direção
escrita antes de qualquer interface existir, um conjunto pequeno de tokens que o código precisa
usar, e checks que rodam fora do modelo.

---

## 1. Regras

### Direção primeiro

Nenhum código de interface é escrito antes de dois artefatos existirem e serem versionados com o
produto:

| Artefato | Contém |
|---|---|
| `design/brief.md` | assunto e domínio, público, a tarefa principal que o usuário vem fazer, duas ou três referências de que o dono gosta e por quê, o único elemento ousado pelo qual o produto será lembrado, e os visuais que o dono veta |
| `design/tokens.json` | a direção como dados, no formato do W3C Design Tokens Community Group 2025.10: cores, tipografia, espaçamento, raio, sombra e movimento |

O código por plataforma (variáveis CSS, Swift, Kotlin, Dart) é **gerado a partir dos tokens** e
nunca editado à mão, pelo mesmo motivo que os arquivos de instrução gerados não são: duas fontes
editáveis de uma decisão não têm resposta para qual delas está certa.

O verificador lê duas extensões do AgenteARQ dentro de `tokens.json`:

- `$extensions["dev.agentarch"].license` em todo token `fontFamily`: a origem e a licença da
  fonte, como `OFL-1.1 via Google Fonts`.
- `$extensions["dev.agentarch"].contrast` na raiz: os pares de primeiro plano e fundo que a
  interface realmente usa, como `[{ "fg": "{color.text}", "bg": "{color.bg}", "use": "text" }]`,
  onde `use` é `text`, `large` ou `ui`.

### Escolhas do dono, com opções

Estas mudam como o produto é percebido e quanto custa, então são do dono, propostas como duas ou
três opções com seus prós e contras e uma recomendação identificada:

- **Tipografia.** No máximo duas famílias com papéis nomeados. Prefira licenças abertas (OFL).
  Uma licença que proíbe redistribuição significa que o arquivo da fonte não pode ser comitado
  num repositório público; registre a licença em qualquer caso.
- **Família de ícones.** Uma família, usada de forma consistente, com a licença registrada. Emoji
  não são ícones.
- **Densidade.** Densa para ferramentas usadas o dia inteiro (dashboards, back offices), mais
  arejada para produtos visitados de vez em quando.

### O processo

1. **Ancore o design no assunto.** Leia o brief: a indústria, seus materiais, seu vocabulário, o
   que seus usuários já reconhecem como confiável. Dois produtos de domínios diferentes não
   deveriam conseguir trocar de interface.
2. **Planeje um conjunto compacto de tokens.** Quatro a seis cores nomeadas, fontes com papéis,
   uma escala de espaçamento, uma ideia de layout. Mais tokens do que decisões é sinal de que nada
   foi decidido.
3. **Revise o plano contra os defaults genéricos.** Compare com os sinais abaixo e com o brief. O
   que parece default de uma biblioteca ou de um modelo é justificado pelo brief ou substituído.
4. **Construa** usando só tokens.
5. **Critique a partir de screenshots** em larguras de celular, tablet e desktop, e então
   **remova um acessório**: a decoração que menos acrescenta.

O brief vence os sinais. Um brief que pede um visual da lista recebe esse visual, e o check
consultivo abaixo é pulado para ele.

### Sinais de visual genérico (lista versão 1)

Esta lista é versionada porque cada geração de modelos produz clichês novos, e uma lista antiga
empurra o modelo direto para o próximo.

| Onda | Sinais |
|---|---|
| 1 | gradientes roxos ou índigo sobre branco; Inter ou Roboto porque ninguém escolheu; um hero centralizado seguido de três cards idênticos com ícone; texto em gradiente; glassmorphism; o mesmo raio e a mesma sombra em tudo; emoji usados como ícones |
| 2 | creme com terracota; quase-preto com um único acento ácido; rótulos em monoespaçada por toda parte; eyebrows em CAIXA ALTA sobre todo título; uma seta em toda chamada para ação; strings de meta `A · B · C` |

### Checks

Checks determinísticos bloqueiam. Juízo estético e tudo que um modelo decide só avisa: um brief
pode pedir legitimamente um visual "proibido", e um juiz LLM nunca é o único bloqueio
(invariante 12).

| Check | O que verifica | Modo |
|---|---|---|
| `design.direction_declared` | `design/brief.md` completo e `design/tokens.json` DTCG válido antes de qualquer interface | `fail_closed` |
| `design.tokens_only` | cor, tamanho, raio e sombra vêm só de tokens ou de suas variáveis | `fail_closed` |
| `design.contrast_wcag2` | todo par de contraste declarado atende WCAG 2 AA: 4,5:1 para texto, 3:1 para texto grande e UI | `fail_closed` |
| `design.a11y_automated` | nenhuma violação automática de acessibilidade serious ou critical em toda página e estado-chave | `fail_closed` |
| `design.focus_and_motion` | foco visível em todo elemento interativo; preferência por movimento reduzido respeitada | `fail_closed` |
| `design.no_placeholder_copy` | sem lorem ipsum, "em breve" ou alt text genérico | `fail_closed` |
| `design.responsive` | sem overflow horizontal nem elementos fixos sobrepostos em 375, 768 e 1280 px | `fail_closed` |
| `design.font_license_recorded` | todo token `fontFamily` nomeia sua origem e licença | `fail_closed` |
| `design.visual_baseline` | diff de screenshot contra a baseline aprovada pelo dono, amarrada ao digest da imagem do verificador | `fail_warn`; o dono aprova uma nova baseline |
| `design.slop_score` | heurísticas de DOM para os sinais acima | `fail_warn`; pulado quando o brief pede o visual |
| `design.apca_advisory` | contraste APCA Lc 75 ou mais em texto corrido | `fail_warn` |
| `design.vision_critique` | crítica de screenshots por um modelo, executada pelo host | `fail_warn`; nunca o único bloqueio |

WCAG 2 AA é a barra exigível. APCA é consultivo porque os rascunhos do WCAG 3 deixaram o método
de contraste em aberto.

---

## 2. O que este padrão não afirma

- Checks automáticos de acessibilidade encontram cerca de metade das barreiras reais. Passar neles
  não é conformidade WCAG, e a evidência nunca deve dizer que é.
- Uma nota baixa de visual genérico não é bom design. Só diz que os defaults comuns foram evitados.
- Gosto não é verificado. O que é verificado é que uma direção foi escolhida pelo dono, escrita,
  usada de forma consistente e mantida acessível.
