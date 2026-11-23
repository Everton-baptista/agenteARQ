package main

import (
	"testing"

	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

// Every advice sentence the workflow exports has a Portuguese translation.
func TestEveryWorkflowAdviceIsTranslated(t *testing.T) {
	prev := lang
	defer func() { lang = prev }()
	lang = "pt-BR"
	for _, a := range []string{workflow.AdviceScenarios, workflow.AdvicePersonas, workflow.AdviceDesign, workflow.AdviceAccept, releaseAdvicePrefix + "release.md needs a section"} {
		if translateAdvice(a) == a {
			t.Errorf("no Portuguese for %q", a)
		}
	}
}

func TestCommonMissingItemsAreTranslated(t *testing.T) {
	defer func(prev string) { lang = prev }(lang)
	lang = "pt-BR"
	for in, want := range map[string]string{
		`agentarch/project/release.md section "Environment" must be filled in without TODO`: `agentarch/project/release.md: preencha a seção "Environment" (sem TODO)`,
		"complete agentarch/project/brief.md":                                               "complete agentarch/project/brief.md",
		"task pedido-form references unknown check e2e":                                     "a tarefa pedido-form cita o check e2e, que este perfil não tem",
		"something new and unforeseen":                                                      "something new and unforeseen",
	} {
		if got := translateMissing(in); got != want {
			t.Errorf("translateMissing(%q) = %q, want %q", in, got, want)
		}
	}
	lang = "ja"
	if got := translateMissing("complete x"); got != "complete x" {
		t.Errorf("a language without a translation changed the item: %q", got)
	}
}
