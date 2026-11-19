package workflow

import (
	"strings"
	"testing"
)

func TestDecisionNeedsComparedOptionsAndAChoiceAmongThem(t *testing.T) {
	if gaps := decisionGaps("adr.md", testADR); len(gaps) != 0 {
		t.Fatalf("a complete ADR was rejected: %v", gaps)
	}
	cases := map[string]struct{ body, want string }{
		"one sentence": {"# ADR\nWe use plain HTML.\n", `needs a "## Options considered" section`},
		"template":     {ADRTemplate, "must be filled in without TODO"},
		"one option": {strings.Replace(testADR, "### Search service\n- Benefits: scales search independently.\n- Costs: a new service to run, secure and monitor.\n", "", 1),
			"at least two options"},
		"no costs": {strings.Replace(testADR, "- Costs: a new service to run, secure and monitor.", "- Also: it is fast to try out today.", 1),
			`option "search service" must state its benefits and its costs`},
		"choice not compared": {strings.Replace(testADR, "Scoped API endpoint, chosen", "A serverless function, chosen", 1),
			"must name the option it chose"},
	}
	for name, c := range cases {
		gaps := strings.Join(decisionGaps("adr.md", c.body), "; ")
		if !strings.Contains(gaps, c.want) {
			t.Errorf("%s: want %q in %q", name, c.want, gaps)
		}
	}
}

// The person writing the ADR writes it in their own language.
func TestDecisionAcceptsPortugueseHeadings(t *testing.T) {
	pt := `# ADR 0001
## Contexto
Site da loja; a equipe só conhece HTML e hospeda em um servidor compartilhado.
## Opções consideradas
### HTML estático
- Benefícios: hospedagem atual, nenhum build, fácil de editar.
- Custos: conteúdo repetido entre páginas.
### Gerador de site estático
- Benefícios: modelos reaproveitáveis.
- Custos: exige Node e um passo de build que a equipe não conhece.
## Decisão
HTML estático, escolhido pela dona da loja pela hospedagem atual.
## Consequências
Se o catálogo passar de 20 páginas, reavaliar o gerador.
`
	if gaps := decisionGaps("adr.md", pt); len(gaps) != 0 {
		t.Fatalf("Portuguese ADR rejected: %v", gaps)
	}
}
