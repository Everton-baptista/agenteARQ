package main

import (
	"fmt"
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
	workflow.AdviceScenarios: "Ligue cada critério de aceite a cenários dado/quando/então em " + workflow.ScenariosPath + "; isso passa a ser obrigatório no content 1.5.",
	workflow.AdvicePersonas:  "Descreva para quem é o produto em " + workflow.PersonasPath + " (objetivo, familiaridade, dispositivo, necessidades de acessibilidade), incluindo um usuário adversário.",
	workflow.AdviceDesign:    "Antes de construir a interface, registre a direção em " + workflow.DesignBriefPath + " e os tokens em " + workflow.DesignTokensPath + " (W3C DTCG), para que pareça este produto e não um padrão.",
	workflow.AdviceAccept:    "Peça ao dono para experimentar o produto e registre com workflow accept.",
}

const releaseAdvicePrefix = "before verifying, so it is reviewed with the code: "

func translateAdvice(advice string) string {
	if lang != "pt-BR" {
		return advice
	}
	if v, ok := workflowAdvicePT[advice]; ok {
		return v
	}
	if strings.HasPrefix(advice, releaseAdvicePrefix) {
		return "antes de verificar, para ser revisado com o código: " + strings.TrimPrefix(advice, releaseAdvicePrefix)
	}
	return advice
}
