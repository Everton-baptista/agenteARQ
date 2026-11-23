package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

// `workflow accept` records the owner trying the product. It is kept apart from `review` on
// purpose: a review asks whether the change is correct, acceptance asks whether it is what the
// owner wanted, and a solo non-technical owner can answer the second even when they cannot
// answer the first.
func acceptProduct(s *workflow.Service, owner, verdict, summary string, expected *uint64) (any, error) {
	return s.Accept(workflow.AcceptRequest{Owner: owner, Verdict: verdict, Summary: summary, ExpectedRevision: expected})
}

func printAcceptance(a *workflow.AcceptanceRecord) {
	if a == nil {
		return
	}
	label, by := "acceptance", "by"
	if lang == "pt-BR" {
		label, by = "aceite", "por"
	}
	fmt.Printf("  %-13s %s %s %s · %s\n", label, a.Verdict, by, a.Owner, a.Summary)
}

// workflowAdvicePT translates the workflow's advice sentences by exact match, like the
// next-step actions. Advice built from a gap keeps the gap in English and translates the frame.
var workflowAdvicePT = map[string]string{
	workflow.AdviceScenarios:  "Ligue cada critério de aceite a cenários dado/quando/então em " + workflow.ScenariosPath + "; isso passa a ser obrigatório no content 1.5.",
	workflow.AdvicePersonas:   "Descreva para quem é o produto em " + workflow.PersonasPath + " (objetivo, familiaridade, dispositivo, necessidades de acessibilidade), incluindo um usuário adversário.",
	workflow.AdviceDesign:     "Antes de construir a interface, registre a direção em " + workflow.DesignBriefPath + " e os tokens em " + workflow.DesignTokensPath + " (W3C DTCG), para que pareça este produto e não um padrão.",
	workflow.AdviceAccept:     "Peça ao dono para experimentar o produto e registre com workflow accept.",
	workflow.AdviceADRSources: "Adicione uma seção \"## Fontes e custo\" a cada ADR em " + workflow.ADRDir + ": a data e a fonte de cada preço ou limite usado, e o custo mensal estimado que o dono aprovou.",
}

const releaseAdvicePrefix = "before verifying, so it is reviewed with the code: "

func translateAdvice(advice string) string {
	if lang != "pt-BR" {
		return advice
	}
	if v, ok := workflowAdvicePT[advice]; ok {
		return v
	}
	if strings.HasPrefix(advice, workflow.RiskAdvicePrefix) {
		return "Risco de segurança (aviso): " + strings.TrimPrefix(advice, workflow.RiskAdvicePrefix)
	}
	if strings.HasPrefix(advice, workflow.DesignAdvicePrefix) {
		body := strings.TrimPrefix(advice, workflow.DesignAdvicePrefix)
		if m := literalColourRe.FindStringSubmatch(body); m != nil {
			body = m[1] + " define " + m[2] + " com a cor literal " + m[3] + "; use uma variável de token para manter a direção registrada."
		}
		return "Design (aviso): " + body
	}
	if strings.HasPrefix(advice, workflow.ReleaseRecommendationPrefix) {
		return "Recomendado antes da produção: preencha " + strings.TrimPrefix(advice, workflow.ReleaseRecommendationPrefix)
	}
	if strings.HasPrefix(advice, releaseAdvicePrefix) {
		return "antes de verificar, para ser revisado com o código: " + strings.TrimPrefix(advice, releaseAdvicePrefix)
	}
	return advice
}

// missingPT translates the most common shapes of a missing item. Each keeps the file names,
// section names and ids exactly as written, since those are what the owner has to find; an item
// with no pattern is shown as written rather than guessed at.
var missingPT = []struct {
	re  *regexp.Regexp
	out string
}{
	{regexp.MustCompile(`^complete (\S+)$`), "complete $1"},
	{regexp.MustCompile(`^(\S+) section "([^"]+)" must be filled in without TODO$`), `$1: preencha a seção "$2" (sem TODO)`},
	{regexp.MustCompile(`^(\S+) needs a "## ([^"]+)" section$`), `$1 precisa de uma seção "## $2"`},
	{regexp.MustCompile(`^(\S+) decision must name the option it chose \(one of: (.*)\)$`), "$1: a decisão precisa nomear a opção escolhida (uma de: $2)"},
	{regexp.MustCompile(`^(\S+) must compare at least two options, each under its own "### " heading$`), `$1 precisa comparar ao menos duas opções, cada uma sob um título "### "`},
	{regexp.MustCompile(`^(\S+) option "([^"]+)" must state its benefits and its costs$`), `$1: a opção "$2" precisa dizer seus benefícios e seus custos`},
	{regexp.MustCompile(`^define at least one task with acceptance criteria and checks$`), "defina ao menos uma tarefa com critérios de aceite e checks"},
	{regexp.MustCompile(`^task (\S+) needs a concrete title$`), "a tarefa $1 precisa de um título concreto"},
	{regexp.MustCompile(`^task (\S+) needs acceptance criteria$`), "a tarefa $1 precisa de critérios de aceite"},
	{regexp.MustCompile(`^task (\S+) references missing acceptance criterion (\S+)$`), "a tarefa $1 cita o critério $2, que não existe em spec.md"},
	{regexp.MustCompile(`^task (\S+) must map acceptance criteria to verification checks$`), "a tarefa $1 precisa ligar seus critérios a checks de verificação"},
	{regexp.MustCompile(`^task (\S+) references unknown check (\S+)$`), "a tarefa $1 cita o check $2, que este perfil não tem"},
	{regexp.MustCompile(`^task (\S+) depends on incomplete (.+)$`), "a tarefa $1 depende de $2, ainda não concluída"},
	{regexp.MustCompile(`^task (\S+)$`), "tarefa $1"},
	{regexp.MustCompile(`^product\.goal must describe the intended outcome without TODO$`), "product.goal precisa descrever o resultado esperado, sem TODO"},
	{regexp.MustCompile(`^product\.author must identify the implementer$`), "product.author precisa identificar quem implementa"},
}

func translateMissing(item string) string {
	if lang != "pt-BR" {
		return item
	}
	for _, p := range missingPT {
		if p.re.MatchString(item) {
			return p.re.ReplaceAllString(item, p.out)
		}
	}
	return item
}

var literalColourRe = regexp.MustCompile(`^(\S+) sets (\S+) to the literal colour (\S+); use a token variable`)
