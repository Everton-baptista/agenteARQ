package workflow

import (
	"os"
	"path/filepath"
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

// A Portuguese project gets Portuguese templates, and its filled-in release plan is accepted.
func TestPortugueseTemplatesAndReleasePlan(t *testing.T) {
	root := repo(t)
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Init(InitOptions{Name: "Loja", Profile: "docs", Goal: "Clientes acham os produtos", Author: "Ana Silva", Language: "pt-BR"}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{documents[0], documents[2], ReleasePlanPath} {
		b, _ := os.ReadFile(filepath.Join(root, rel))
		if !strings.Contains(string(b), "## ") || strings.Contains(string(b), "## Context\n") || strings.Contains(string(b), "## Environment") {
			t.Errorf("%s is not the Portuguese template", rel)
		}
	}
	plan := "# Plano\n## Ambiente\nHospedagem compartilhada da loja.\n## Responsável\nAna Silva responde pelo release.\n## Reversão\nReenviar os arquivos da versão anterior.\n## Verificação pós-deploy\nAbrir a página inicial e o contato.\n## Sinais\nLog de erros e mensagens de clientes por uma semana.\n"
	if err := os.WriteFile(filepath.Join(root, ReleasePlanPath), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	if gaps := s.releasePlanGaps(); len(gaps) != 0 {
		t.Fatalf("Portuguese release plan rejected: %v", gaps)
	}
}
