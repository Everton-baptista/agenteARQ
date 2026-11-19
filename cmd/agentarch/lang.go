package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Everton-baptista/agenteARQ/internal/blueprint"
)

// The interview's own words, in every language it speaks.
//
// This is a lookup table and nothing more. `golang.org/x/text/message` would bring plurals, gender
// and number formatting; the interview asks four questions and has no use for any of it, and a
// catalogue that only looks strings up is one that stays honest without a maintenance ritual.
//
// Scope is deliberately narrow: the interview, and the text that closes it. Gate findings,
// `explain`, `report` and error messages stay in English. Translating those would put a
// translation obligation on every new control, and a stale translation of a rule answers the
// reader's question with authority using a rule that has since changed — the failure
// `AA-I18N-016` exists to catch in the standards, reproduced in the tool.

// SourceLang is the language every string is written in first. A key missing from another
// language falls back to this one, so a half-finished translation degrades to English rather than
// to a blank line.
const SourceLang = "en"

// languages are what the first question offers, in the order it offers them.
var languages = []struct {
	Code string // what --lang takes, and what the content tree is keyed by
	Name string // shown in its own language: somebody choosing Portuguese reads Portuguese
}{
	{"en", "English"},
	{"pt-BR", "Português (Brasil)"},
}

// lang is the chosen language for this run. It is a package variable because the alternative is
// threading a locale through every print site in the interview, which is how a translation gets
// half-applied — one function keeps its English because nobody passed it the parameter.
var lang = SourceLang

// t looks up a key in the chosen language.
//
// A missing key returns the English, and a key missing from English returns the key itself
// bracketed, which is loud enough to be caught in review and harmless enough not to crash an
// interview over a typo. TestEveryKeyExistsInEveryLanguage makes both cases test failures.
func t(key string) string {
	if s, ok := catalog[lang][key]; ok {
		return s
	}
	if s, ok := catalog[SourceLang][key]; ok {
		return s
	}
	return "[" + key + "]"
}

// tf is t with formatting.
func tf(key string, args ...any) string { return fmt.Sprintf(t(key), args...) }

// askLanguage is the first question, and the only one that cannot be asked in the chosen
// language — so it is asked in both.
//
// It comes before the banner: a greeting has to be written in some language, and choosing one
// before asking is how a tool tells somebody it was not built for them.
func askLanguage() (code string, quit bool) {
	fmt.Printf("\n%s\n\n", "Language / Idioma")
	for i, l := range languages {
		fmt.Printf("  %d. %s\n", i+1, l.Name)
	}
	fmt.Println()

	for attempt := 0; attempt < 3; attempt++ {
		in := strings.ToLower(strings.TrimSpace(ask("Choose / Escolha 1–2 [1]: ")))
		switch in {
		case "":
			return languages[0].Code, false
		case "q", "quit", "sair":
			fmt.Println("nothing written / nada foi escrito")
			return "", true
		}
		// By number, by code, or by name — somebody who types "pt" or "português" meant it.
		for i, l := range languages {
			if in == fmt.Sprint(i+1) || in == strings.ToLower(l.Code) ||
				strings.HasPrefix(strings.ToLower(l.Name), in) {
				return l.Code, false
			}
		}
		if in == "pt" || in == "br" || in == "portugues" {
			return "pt-BR", false
		}
		fmt.Fprintln(os.Stderr, "  not one of the options / não é uma das opções")
	}
	return languages[0].Code, false
}

// blueprintTitle and blueprintNeed read the catalogue entry in the chosen language.
//
// They exist so no print site has to remember to pass `lang`, which is how half a menu ends up
// translated — the two rows somebody forgot stay in English and read like a bug in the content.
func blueprintTitle(b blueprint.Blueprint) string { return b.Meta.TitleIn(lang) }
func blueprintNeed(b blueprint.Blueprint) string  { return b.Meta.NeedIn(lang) }

// langSet reports whether --lang was passed rather than left at its default.
//
// The distinction matters: a script that names a language must not be interrupted by a question,
// and a person who named one already answered it. Comparing against the default value would be
// wrong — somebody passing `--lang en` explicitly has answered, and should not be asked.
func langSet(fs_ *flag.FlagSet) bool {
	set := false
	fs_.Visit(func(f *flag.Flag) {
		if f.Name == "lang" {
			set = true
		}
	})
	return set
}

// knownLanguage reports whether a --lang value is one the interview speaks.
func knownLanguage(code string) bool {
	for _, l := range languages {
		if l.Code == code {
			return true
		}
	}
	return false
}

// languageCodes is every code, for error messages and tests.
func languageCodes() []string {
	out := make([]string, 0, len(languages))
	for _, l := range languages {
		out = append(out, l.Code)
	}
	sort.Strings(out)
	return out
}

// catalog is keyed by language, then by message key.
//
// Keys are dotted and describe the place, not the text: renaming a sentence must not orphan its
// translations. Anything with a %s carries the same verbs in the same order in every language —
// the test checks the count, which catches the reordering that silently prints the wrong value.
var catalog = map[string]map[string]string{

	"en": {
		// ---- the banner
		"banner.title": "agentarch — let's get you set up.",
		"banner.body": "A few questions in plain language; nothing is written until you say so.\n" +
			"Where a choice is yours, the options are explained and none is picked for you. q quits.",

		// ---- question: does the code already exist
		"existing.question":     "What do you want to do?",
		"existing.product":      "Build a software product of any kind — site, API, app, CLI, docs, data, or an agent from scratch — step by step, with technology options compared before you choose",
		"existing.new":          "Start a new AI agent from a complete project that already works, so I can edit it",
		"existing.adopt":        "Continue something already built — describe what is here (agent or not) and carry on",
		"existing.refactor":     "Refactor an existing AI agent to the standard, with tests and review",
		"existing.hint.empty":   "this directory looks empty",
		"existing.hint.hascode": "there is already code here",
		"existing.prompt":       "Choose 1–4: ",

		// ---- question: what are you building
		"blueprint.question": "What are you building?",
		"blueprint.runson":   "runs on %s",
		"blueprint.prompt":   "Choose 1–%d (or q to quit): ",

		// ---- question: which framework
		"framework.question":   "This blueprint ships runnable code for:",
		"framework.none":       "(the provider's SDK called directly from Python: fewest dependencies, you write the loop)",
		"framework.label.none": "no framework (plain Python)",
		"framework.prompt":     "Choose 1–%d: ",

		// ---- question: which model provider
		"provider.question": "Which model provider will it call?",
		"provider.explain": "All of them are wired up — the agent code is the same either way; only the\n" +
			"manifest and one pinned SDK change. Compare them on what matters to you: answer\n" +
			"quality on your own examples, price, latency, where data is processed, and which\n" +
			"account your organisation already has. The model id is pinned, not an alias.",
		"provider.prompt": "Choose 1–%d: ",

		// ---- question: where are the users
		"juris.question":    "Where are the people who will use it?",
		"juris.brazil":      "Brazil",
		"juris.brazil.note": "brings the LGPD rules in",
		"juris.europe":      "Europe",
		"juris.europe.note": "brings GDPR and the EU AI Act in",
		"juris.both":        "Both",
		"juris.other":       "Somewhere else",
		"juris.other.note":  "type the country code, e.g. US, IN, JP, NG",
		"juris.undecided":   "Not decided yet",
		"juris.explain": "Only Brazil and Europe have legal packs written so far. Declaring any other\n" +
			"country costs nothing today and starts applying the day its pack exists —\n" +
			"the standards themselves are the same everywhere.",
		"juris.prompt":   "Choose, or type country codes [5]: ",
		"juris.typecode": "  type the code itself, e.g. US — or US,CA for more than one",
		"juris.notacode": "  not an option, and not a two-letter country code",

		// ---- the plan, shown before anything is written
		"plan.adopting":        "Adopting the agents that are already here.",
		"plan.refactoring":     "Adopting what is here, and installing the refactoring workflow.",
		"plan.runningon":       "running on %s",
		"plan.installsinto":    "installs into",
		"plan.strictness":      "strictness",
		"plan.strictness.note": "(change it later in agentarch/agentarch.yaml)",
		"plan.provider":        "model",
		"plan.accountable":     "accountable",
		"plan.contact":         "contact",
		"plan.regulation":      "regulation",
		"plan.writes":          "writes",
		"plan.writes.files":    "%d file(s), including runnable code under app/",
		"plan.confirm":         "\nGo ahead?",
		"plan.nothing":         "nothing written",

		// ---- the product path
		"product.ask.goal":        "\nWhat will people be able to do with it? One sentence, in your words:\n> ",
		"product.ask.owner":       "\nWho is accountable for it? A person's name, not a team:\n> ",
		"product.needflag":        "start --product: pass --%s when not on a terminal",
		"product.needprofile":     "start --product: pass --engineering-profile; none is chosen for you:",
		"product.plan.name":       "product",
		"product.plan.goal":       "goal",
		"product.plan.owner":      "accountable",
		"product.plan.profile":    "verified as",
		"product.plan.tech":       "technology",
		"product.plan.tech.value": "not chosen yet: options are compared with you in the ADR",
		"product.notgit":          "The workflow tracks a Git repository: run `git init` here and try again.",
		"product.done": "\nDone. The standards are in agentarch/std and the product workflow is open.\n\n" +
			"What happens next, in order — each step leaves a file in agentarch/project/ and the\n" +
			"next one stays closed until it exists:\n\n" +
			"  1. brief.md     who it is for, the problem, success, constraints (including any\n" +
			"                  technology you require or rule out — or \"no preference\")\n" +
			"  2. spec.md      acceptance criteria AC-1, AC-2… observable from outside\n" +
			"  3. adr/0001     two or more technology options compared on benefits and costs;\n" +
			"                  you choose, and the ADR records why\n" +
			"  4. tasks.yaml   work mapped to acceptance criteria and checks\n" +
			"  5. release.md   environment, owner, rollback and post-deploy checks, reviewed with the code\n" +
			"  6. verify       isolated checks plus the security floor (agentarch workflow\n" +
			"                  prepare-verifier builds the image once), then independent review,\n" +
			"                  release and a production check\n\n" +
			"Do it with your assistant: agentarch plugin install <claude|codex|gemini> ...\n" +
			"then ask it to \"start the product\". It will propose options rather than pick\n" +
			"them. Or by hand: agentarch workflow status shows the next step at any time.\n",

		// ---- choosing a verification profile
		"profile.question": "\nHow should this product be verified? Each profile runs different checks:\n",
		"profile.legend":   "\n  * = what the files here suggest. Every profile also runs the security floor.\n",
		"profile.prompt":   "Choose 1–%d, or type the id: ",

		// ---- adopting a project that is not an agent
		"profile.checks": "checks:",
		"notagent.head": "\nNo agent here: no model SDK or agent framework is declared or imported, so no\n" +
			"agent.yaml was written. That is fine. The standards under agentarch/std apply\n" +
			"to any product, and the product workflow takes it from spec to production with\n" +
			"isolated verification. Every profile adds the same security floor (secrets and\n" +
			"known-vulnerable dependencies). Pick the one that fits:\n\n",
		"notagent.legend": "\n  * = what this project's files suggest; nothing is chosen for you.\n",
		"notagent.then": "\nThen:  agentarch start --product   (or agentarch workflow init --name <product> --profile <id> \\\n" +
			"         --goal \"<the user outcome>\" --author \"<your name>\")\n\n" +
			"If this project does call a model in a way the scan cannot see (a raw HTTP\n" +
			"call, say), describe it with:  agentarch init --adopt --adopt-id <id> --agent\n",

		// ---- workflow status
		"workflow.status": "Phase: %s · revision %d\n\nNext step: %s",

		// ---- shared
		"common.notanoption": "  not one of the options",
		"common.givingup":    "giving up after three tries; nothing written",
	},

	"pt-BR": {
		"banner.title": "agentarch — vamos configurar seu projeto.",
		"banner.body": "Algumas perguntas em linguagem simples. Nada será escrito até você confirmar.\n" +
			"Quando a escolha é sua, as opções são explicadas e nenhuma é escolhida por você. q sai.",

		"existing.question":     "O que você quer fazer?",
		"existing.product":      "Construir um produto de software de qualquer tipo — site, API, app, CLI, documentação, dados ou um agente do zero — passo a passo, comparando opções de tecnologia antes de você escolher",
		"existing.new":          "Começar um agente de IA a partir de um projeto completo que já funciona",
		"existing.adopt":        "Continuar algo que já existe — descrever o que há aqui (agente ou não) e seguir",
		"existing.refactor":     "Refatorar um agente de IA existente seguindo o padrão, com testes e revisão",
		"existing.hint.empty":   "este diretório parece vazio",
		"existing.hint.hascode": "já existe código aqui",
		"existing.prompt":       "Escolha 1–4: ",

		"blueprint.question": "O que você quer construir?",
		"blueprint.runson":   "roda com %s",
		"blueprint.prompt":   "Escolha 1–%d (ou q para sair): ",

		"framework.question":   "Este blueprint traz código que roda para:",
		"framework.none":       "(o SDK do provedor chamado direto do Python: menos dependências, você escreve o laço)",
		"framework.label.none": "sem framework (Python puro)",
		"framework.prompt":     "Escolha 1–%d: ",

		"provider.question": "Qual provedor de modelo ele vai chamar?",
		"provider.explain": "Todos já estão prontos — o código do agente é o mesmo em qualquer um; só mudam\n" +
			"o manifesto e um SDK fixado. Compare pelo que importa para você: qualidade nas\n" +
			"suas próprias perguntas, preço, latência, onde os dados são processados e qual\n" +
			"conta a sua organização já tem. O id do modelo é fixado, não um alias.",
		"provider.prompt": "Escolha 1–%d: ",

		"juris.question":    "Onde seu projeto vai ser usado?",
		"juris.brazil":      "Brasil",
		"juris.brazil.note": "traz as regras da LGPD",
		"juris.europe":      "Europa",
		"juris.europe.note": "traz o GDPR e o AI Act da UE",
		"juris.both":        "Ambos",
		"juris.other":       "Outro país",
		"juris.other.note":  "informe o código, por exemplo US, IN, JP, NG",
		"juris.undecided":   "Ainda não decidi",
		"juris.explain": "Só Brasil e Europa têm pacotes legais escritos até agora. Declarar outro\n" +
			"país não custa nada hoje e passa a valer no dia em que o pacote existir —\n" +
			"os padrões em si são os mesmos em qualquer lugar.",
		"juris.prompt":   "Escolha, ou digite os códigos de país [5]: ",
		"juris.typecode": "  digite o código, por exemplo US — ou US,CA para mais de um",
		"juris.notacode": "  não é uma opção, e não é um código de país de duas letras",

		"plan.adopting":        "Descrevendo os agentes que já estão aqui.",
		"plan.refactoring":     "Descrevendo o que há aqui, e instalando o fluxo de refatoração.",
		"plan.runningon":       "rodando com %s",
		"plan.installsinto":    "instala em",
		"plan.strictness":      "rigor",
		"plan.strictness.note": "(mude depois em agentarch/agentarch.yaml)",
		"plan.provider":        "modelo",
		"plan.accountable":     "responsável",
		"plan.contact":         "contato",
		"plan.regulation":      "regulação",
		"plan.writes":          "escreve",
		"plan.writes.files":    "%d arquivo(s), incluindo código que roda em app/",
		"plan.confirm":         "\nPodemos seguir?",
		"plan.nothing":         "nada foi escrito",

		"product.ask.goal":        "\nO que as pessoas vão conseguir fazer com ele? Uma frase, com as suas palavras:\n> ",
		"product.ask.owner":       "\nQuem é o responsável? O nome de uma pessoa, não de um time:\n> ",
		"product.needflag":        "start --product: passe --%s quando não houver terminal",
		"product.needprofile":     "start --product: passe --engineering-profile; nenhum é escolhido por você:",
		"product.plan.name":       "produto",
		"product.plan.goal":       "objetivo",
		"product.plan.owner":      "responsável",
		"product.plan.profile":    "verificado como",
		"product.plan.tech":       "tecnologia",
		"product.plan.tech.value": "ainda não escolhida: as opções são comparadas com você no ADR",
		"product.notgit":          "O fluxo acompanha um repositório Git: rode `git init` aqui e tente de novo.",
		"product.done": "\nPronto. Os padrões estão em agentarch/std e o fluxo do produto está aberto.\n\n" +
			"O que vem a seguir, em ordem — cada etapa deixa um arquivo em agentarch/project/ e a\n" +
			"próxima só abre quando ele existe:\n\n" +
			"  1. brief.md     para quem é, o problema, o sucesso, as restrições (inclusive\n" +
			"                  tecnologias exigidas ou proibidas — ou \"sem preferência\")\n" +
			"  2. spec.md      critérios de aceite AC-1, AC-2… observáveis de fora\n" +
			"  3. adr/0001     duas ou mais opções de tecnologia comparadas por benefícios e custos;\n" +
			"                  você escolhe, e o ADR registra o porquê\n" +
			"  4. tasks.yaml   o trabalho ligado aos critérios de aceite e aos checks\n" +
			"  5. release.md   ambiente, responsável, rollback e checagens pós-deploy, revisados com o código\n" +
			"  6. verify       checks isolados mais o piso de segurança (agentarch workflow\n" +
			"                  prepare-verifier constrói a imagem uma vez), depois revisão\n" +
			"                  independente, release e verificação em produção\n\n" +
			"Faça com o seu assistente: agentarch plugin install <claude|codex|gemini> ...\n" +
			"e peça para ele \"começar o produto\". Ele vai propor opções em vez de escolher\n" +
			"por você. Ou à mão: agentarch workflow status mostra o próximo passo a qualquer momento.\n",
		"profile.question": "\nComo este produto deve ser verificado? Cada perfil roda checks diferentes:\n",
		"profile.legend":   "\n  * = o que os arquivos daqui sugerem. Todo perfil também roda o piso de segurança.\n",
		"profile.prompt":   "Escolha 1–%d, ou digite o id: ",
		"profile.checks":   "verifica:",
		"notagent.head": "\nNenhum agente aqui: nenhum SDK de modelo ou framework de agente é declarado ou\n" +
			"importado, então nenhum agent.yaml foi escrito. Tudo bem. Os padrões em agentarch/std\n" +
			"valem para qualquer produto, e o fluxo do produto o leva da especificação à produção\n" +
			"com verificação isolada. Todo perfil traz o mesmo piso de segurança (segredos e\n" +
			"dependências com vulnerabilidades conhecidas). Escolha o que se encaixa:\n\n",
		"notagent.legend": "\n  * = o que os arquivos deste projeto sugerem; nada é escolhido por você.\n",
		"notagent.then": "\nDepois:  agentarch start --product   (ou agentarch workflow init --name <produto> --profile <id> \\\n" +
			"           --goal \"<o resultado para o usuário>\" --author \"<seu nome>\")\n\n" +
			"Se este projeto chama um modelo de um jeito que o scan não enxerga (uma chamada\n" +
			"HTTP direta, por exemplo), descreva-o com:  agentarch init --adopt --adopt-id <id> --agent\n",
		"workflow.status":    "Fase: %s · revisão %d\n\nPróximo passo: %s",
		"common.notanoption": "  não é uma das opções",
		"common.givingup":    "desisti após três tentativas; nada foi escrito",
	},
}

// projectLanguage is the language the project's generated instruction files were written in,
// read from their header, so the workflow speaks to the person in the language they chose.
func projectLanguage(root string) string {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		if m := generatedLangRe.FindSubmatch(b); m != nil && knownLanguage(string(m[1])) {
			return string(m[1])
		}
	}
	return SourceLang
}

var generatedLangRe = regexp.MustCompile(`agentarch:generated[^\n]*\blang=([A-Za-z-]+)`)

// workflowActionsPT translates the workflow's next-step sentences. A sentence with no entry is
// shown as written, so a new action is never hidden by a missing translation.
var workflowActionsPT = map[string]string{
	"Complete the brief and specification; checkpoint specified.":                                                                                                         "Complete o brief e a especificação; registre o checkpoint specified.",
	"Record the architecture decision and acceptance-to-check task mapping; checkpoint planned.":                                                                          "Registre a decisão de arquitetura (opções comparadas) e as tarefas ligadas aos critérios e checks; registre o checkpoint planned.",
	"Use a dedicated worktree and begin implementation; checkpoint implementing.":                                                                                         "Use um worktree dedicado e comece a implementação; registre o checkpoint implementing.",
	"Resolve the unfinished task dependencies before continuing implementation.":                                                                                          "Resolva as dependências de tarefas pendentes antes de continuar a implementação.",
	"All planned tasks are complete; run verification.":                                                                                                                   "Todas as tarefas planejadas estão concluídas; rode a verificação.",
	"Run the engineering profile in an isolated container; fix failed or inconclusive checks.":                                                                            "Rode o perfil de engenharia em um contêiner isolado; corrija os checks com falha ou inconclusivos.",
	"Request an independent human or agent review of this exact fingerprint.":                                                                                             "Peça uma revisão independente, humana ou de agente, deste fingerprint exato.",
	"Resolve the recorded blocker and checkpoint implementing.":                                                                                                           "Resolva o bloqueio registrado e registre o checkpoint implementing.",
	"Work cancelled; explicitly checkpoint draft to reopen.":                                                                                                              "Trabalho cancelado; registre explicitamente o checkpoint draft para reabrir.",
	"The local gate passed. Publish the PR with your host; after a person or CI deploys this content, record it with record-release. This tool does not merge or deploy.": "O gate local passou. Publique o PR pelo seu host; depois que uma pessoa ou o CI fizer o deploy deste conteúdo, registre com record-release. Esta ferramenta não faz merge nem deploy.",
	"The local gate passed. Publish the PR with your host; after a person or CI deploys this content, record it with record-release. This tool does not merge or deploy. Releasing also needs agentarch/project/release.md in the reviewed source.": "O gate local passou. Publique o PR pelo seu host; depois que uma pessoa ou o CI fizer o deploy deste conteúdo, registre com record-release. Esta ferramenta não faz merge nem deploy. O release também precisa de agentarch/project/release.md no código revisado.",
	"Run the post-deploy verification from agentarch/project/release.md and record it with record-production-check.":                                                                                                                                "Faça a verificação pós-deploy descrita em agentarch/project/release.md e registre com record-production-check.",
	"Shipped and checked in production. Keep watching the signals in agentarch/project/release.md; new work starts at implementing.":                                                                                                                "Em produção e verificado. Continue acompanhando os sinais de agentarch/project/release.md; um trabalho novo começa em implementing.",
}

func translateAction(action string) string {
	if lang != "pt-BR" {
		return action
	}
	if v, ok := workflowActionsPT[action]; ok {
		return v
	}
	if strings.HasPrefix(action, "Implement task ") && strings.HasSuffix(action, " and checkpoint it when complete.") {
		id := strings.TrimSuffix(strings.TrimPrefix(action, "Implement task "), " and checkpoint it when complete.")
		return "Implemente a tarefa " + id + " e registre o checkpoint quando terminar."
	}
	return action
}
