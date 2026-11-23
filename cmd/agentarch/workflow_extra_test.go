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
