# Roteiro do facilitador

Use este roteiro em cada sessão. O objetivo é descobrir onde o AgenteARQ deixa uma pessoa leiga
sem saber o que fazer, então o facilitador **não ensina a ferramenta**.

## Antes da sessão

- [ ] Termo de consentimento assinado (`termo-de-consentimento.pt-BR.md`).
- [ ] Máquina limpa com Git, Podman ou Docker, e o assistente instalado e logado na conta do
      projeto (nunca na conta do participante).
- [ ] `agentarch version` anotado no registro.
- [ ] Pasta vazia para o projeto. Nada mais preparado.
- [ ] `pilot/<id>/log.csv` criado a partir de `log-template.csv`.
- [ ] Gravação de tela e áudio ligada.

## O que ler em voz alta, exatamente

> Construa o que você descreveu, junto com o assistente, até que pessoas de verdade possam usar.
> Pense em voz alta. Eu não posso ajudar com a ferramenta. Se em casa você pararia aqui, diga
> "eu pararia" e a gente anota.

Depois disso, fique em silêncio sobre a ferramenta. Responda só a perguntas sobre a sessão
(tempo, pausa, banheiro).

## Quando intervir

Só em dois casos, e sempre registrando uma linha `intervention` com o motivo:

1. A pessoa disse "eu pararia" **duas vezes** no mesmo obstáculo.
2. Algo inseguro vai acontecer: colar uma senha ou chave real, fazer um pagamento, apagar algo
   fora da pasta do projeto.

Ao intervir, diga o mínimo que destrava e volte ao silêncio.

## O que registrar (uma linha por evento)

| Situação | kind | step | words |
|---|---|---|---|
| "Eu pararia" | `stop` | onde (ex.: `doctor`, `brief`, `verify`) | as palavras que estavam na tela |
| Você ajudou | `intervention` | onde | o que você disse |
| Uma recusa apareceu e a explicação não bastou | `unclear_refusal` | o id em `agentarch explain` | o que a pessoa entendeu |
| Gate burlado, arquivo gerado ou evidência editados à mão | `bypass` | onde | o que foi feito |
| Revisão registrada em nome de quem não revisou | `fabricated_reviewer` | `review` | o nome usado |
| Qualquer outra coisa útil | `note` | onde | o quê |

## Ao fim de cada sessão

```bash
python3 scripts/pilot.py collect --product <pasta do projeto> --participant P01 --session 1 --out pilot
```

Confira que a gravação ficou só na pasta do piloto e anote a data de exclusão (90 dias).

## Ao fim do grupo

```bash
python3 scripts/pilot.py summarize --out pilot
```

O resultado diz quantos chegaram a `production_verified` sem ajuda, se houve quebra de
integridade e quais obstáculos pararam mais de uma pessoa. Cada obstáculo repetido vira uma
correção antes do próximo grupo, sempre com participantes novos.
