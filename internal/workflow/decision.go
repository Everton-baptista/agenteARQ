package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// A technology decision is only as good as the alternatives it was weighed against. An ADR that
// says "we use X" records a preference, not a decision, and the person reading it later cannot
// tell whether anything else was considered. So every ADR must compare at least two options,
// each with what it buys and what it costs, and the decision must pick one of those options.
// This is checked on the file, not trusted to the model that wrote it.

const ADRDir = "agentarch/project/adr"

// Headings are accepted in the languages the interview speaks, because the person writing the
// ADR should write it in their own language.
var (
	adrContext      = []string{"context", "contexto"}
	adrOptions      = []string{"options considered", "options", "alternatives", "opções consideradas", "opções", "alternativas", "opciones consideradas", "opciones"}
	adrDecision     = []string{"decision", "decisão", "decisión"}
	adrConsequences = []string{"consequences", "consequências", "consecuencias"}
	benefitWords    = []string{"benefit", "pros", "advantage", "strength", "benefício", "beneficio", "vantage", "ventaja", "pontos fortes"}
	costWords       = []string{"cost", "cons", "drawback", "trade-off", "tradeoff", "risk", "custo", "desvantage", "contra", "desventaja", "risco", "riesgo"}
)

// markdownSections splits a document by headings of the given level ("## " or "### ").
func markdownSections(body, prefix string) (order []string, sections map[string]string) {
	sections = map[string]string{}
	current := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, prefix) {
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
			if _, seen := sections[current]; !seen {
				order = append(order, current)
			}
			sections[current] += ""
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	return order, sections
}

func findSection(sections map[string]string, names []string) (string, bool) {
	for _, n := range names {
		if body, ok := sections[n]; ok {
			return body, true
		}
	}
	return "", false
}

func mentionsAny(text string, words []string) bool {
	lower := strings.ToLower(text)
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// decisionGaps lists what one ADR is missing.
func decisionGaps(rel, body string) []string {
	_, sections := markdownSections(body, "## ")
	var gaps []string
	need := func(names []string, label string) (string, bool) {
		text, ok := findSection(sections, names)
		switch {
		case !ok:
			gaps = append(gaps, fmt.Sprintf("%s needs a \"## %s\" section", rel, label))
		case !meaningful(text):
			gaps = append(gaps, fmt.Sprintf("%s section %q must be filled in without TODO", rel, label))
		default:
			return text, true
		}
		return "", false
	}
	need(adrContext, "Context")
	options, haveOptions := need(adrOptions, "Options considered")
	decision, haveDecision := need(adrDecision, "Decision")
	need(adrConsequences, "Consequences")

	var titles []string
	if haveOptions {
		order, opts := markdownSections(options, "### ")
		for _, title := range order {
			text := opts[title]
			if !meaningful(text) || !mentionsAny(text, benefitWords) || !mentionsAny(text, costWords) {
				gaps = append(gaps, fmt.Sprintf("%s option %q must state its benefits and its costs", rel, title))
				continue
			}
			titles = append(titles, title)
		}
		if len(order) < 2 {
			gaps = append(gaps, fmt.Sprintf("%s must compare at least two options, each under its own \"### \" heading", rel))
		}
	}
	if haveOptions && haveDecision && len(titles) > 0 {
		chosen := false
		said := normalizeOption(decision)
		for _, t := range titles {
			chosen = chosen || namesAffirmatively(said, normalizeOption(t))
		}
		if !chosen {
			gaps = append(gaps, fmt.Sprintf("%s decision must name the option it chose (one of: %s)", rel, strings.Join(titles, ", ")))
		}
	}
	return gaps
}

// architectureGaps checks every ADR in the project; the first one must exist.
func (s *Service) architectureGaps() []string {
	dir, e := safePath(s.Root, ADRDir)
	if e != nil {
		return []string{e.Error()}
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, en := range entries {
		if !en.IsDir() && strings.HasSuffix(en.Name(), ".md") {
			names = append(names, en.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return []string{"complete " + documents[2]}
	}
	var gaps []string
	for _, n := range names {
		rel := filepath.ToSlash(filepath.Join(ADRDir, n))
		b, e := os.ReadFile(filepath.Join(dir, n))
		if e != nil {
			gaps = append(gaps, "cannot read "+rel)
			continue
		}
		gaps = append(gaps, decisionGaps(rel, string(b))...)
	}
	return gaps
}

// ADRTemplate is the first ADR a new product gets. Its TODO markers keep it from passing until a
// person has compared real options for this product.
const ADRTemplate = `# ADR 0001 — Architecture

## Context

TODO: what the product must do (from the brief and spec), the constraints that matter (team
skills, budget, hosting, data, compliance, deadline) and what already exists in this repository.

## Options considered

Compare at least two options that genuinely fit this demand. For each, say what it buys and what
it costs, in terms of the constraints above. Keep the current stack as an option when there is one.

### TODO first option

- Benefits: TODO
- Costs: TODO

### TODO second option

- Benefits: TODO
- Costs: TODO

## Decision

TODO: name the option chosen, who chose it, and why it wins for this product.

## Consequences

TODO: what becomes easier, what becomes harder, and what would make you revisit this decision.

## Sources and cost

TODO: the date and source of every price or limit used above, and the estimated monthly cost the
owner approved.
`

// BriefTemplate and SpecTemplate guide a person who has never written one. Technology is asked
// about as a constraint, not chosen: the choice belongs to the ADR, after options are compared.
const BriefTemplate = `# Product brief

## Users

TODO: who uses this, and in what situation.

## Problem

TODO: what they cannot do today, in their words.

## Success measure

TODO: an observable result that tells you it worked (for example, a task done in one step).

## Constraints

TODO: deadline, budget, hosting, data and privacy rules, accessibility, and the team's skills.
Technology preferences or mandates go here as constraints (for example "must run on our existing
PHP hosting" or "no preference"); the technology itself is chosen in the ADR, after comparing options.

## Out of scope

TODO: what this product will deliberately not do.
`

const SpecTemplate = `# Specification

Each acceptance criterion is observable from outside the code and has an id that tasks refer to.

- AC-1: TODO
- AC-2: TODO
`

func templateFor(language, en, pt string) string {
	if language == "pt-BR" {
		return pt
	}
	return en
}

const BriefTemplatePT = `# Brief do produto

## Usuários

TODO: quem usa, e em que situação.

## Problema

TODO: o que essas pessoas não conseguem fazer hoje, nas palavras delas.

## Medida de sucesso

TODO: um resultado observável que mostre que funcionou (por exemplo, uma tarefa feita em um passo).

## Restrições

TODO: prazo, orçamento, hospedagem, regras de dados e privacidade, acessibilidade e o que a equipe
já sabe. Preferências ou exigências de tecnologia entram aqui como restrição (por exemplo "precisa
rodar na hospedagem PHP atual" ou "sem preferência"); a tecnologia em si é escolhida no ADR, depois
de comparar opções.

## Fora do escopo

TODO: o que este produto deliberadamente não vai fazer.
`

const SpecTemplatePT = `# Especificação

Cada critério de aceite é observável de fora do código e tem um id que as tarefas usam.

- AC-1: TODO
- AC-2: TODO
`

const ADRTemplatePT = `# ADR 0001 — Arquitetura

## Contexto

TODO: o que o produto precisa fazer (do brief e da especificação), as restrições que importam
(conhecimento da equipe, orçamento, hospedagem, dados, conformidade, prazo) e o que já existe aqui.

## Opções consideradas

Compare pelo menos duas opções que realmente atendam à demanda. Para cada uma, diga o que ela traz e
o que ela custa, nos termos das restrições acima. Se já existe uma stack, mantê-la é uma das opções.

### TODO primeira opção

- Benefícios: TODO
- Custos: TODO

### TODO segunda opção

- Benefícios: TODO
- Custos: TODO

## Decisão

TODO: a opção escolhida, quem escolheu, e por que ela vence para este produto.

## Consequências

TODO: o que fica mais fácil, o que fica mais difícil, e o que faria você rever esta decisão.

## Fontes e custo

TODO: a data e a fonte de cada preço ou limite usado acima, e o custo mensal estimado que o dono
aprovou.
`

var parenthetical = regexp.MustCompile(`\([^)]*\)`)

// normalizeOption lets a decision name an option the way a person writes it: case, punctuation
// and a parenthetical detail in the option's heading ("Static site (HTML, CSS and JavaScript)")
// do not have to be repeated word for word. The option's words still have to appear, in order.
func normalizeOption(s string) string {
	s = parenthetical.ReplaceAllString(strings.ToLower(s), " ")
	fields := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	return " " + strings.Join(fields, " ") + " "
}

var negations = map[string]bool{"not": true, "no": true, "não": true, "nao": true, "sem": true, "without": true, "rejected": true, "rejeitado": true, "rejeitada": true, "nunca": true, "never": true, "ni": true, "sin": true}

// namesAffirmatively finds the option in the decision, ignoring a mention right after a negation
// ("not Postgres", "não Postgres"), which names an option only to rule it out.
func namesAffirmatively(said, option string) bool {
	for from := 0; ; {
		i := strings.Index(said[from:], option)
		if i < 0 {
			return false
		}
		at := from + i
		before := strings.Fields(said[:at])
		if len(before) == 0 || !negations[before[len(before)-1]] {
			return true
		}
		from = at + 1
	}
}
