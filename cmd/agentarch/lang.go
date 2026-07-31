package main

import (
	"flag"
	"fmt"
	"os"
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
			"Press Enter to take the default, or q to quit.",

		// ---- question: does the code already exist
		"existing.question":     "Is this a new agent, or does the code already exist?",
		"existing.new":          "New — start me off with a complete project that works, so I can edit it",
		"existing.adopt":        "Already built — describe what is here and continue from it",
		"existing.refactor":     "Already built — refactor it to the standard, with tests and review",
		"existing.hint.empty":   "this directory looks empty",
		"existing.hint.hascode": "there is already code here",
		"existing.prompt":       "Choose 1–3 [%d]: ",

		// ---- question: what are you building
		"blueprint.question": "What are you building?",
		"blueprint.runson":   "runs on %s",
		"blueprint.prompt":   "Choose 1–%d (or q to quit): ",

		// ---- question: which framework
		"framework.question":   "This blueprint ships runnable code for:",
		"framework.none":       "(the provider SDK directly)",
		"framework.label.none": "no framework",
		"framework.prompt":     "Choose 1–%d [1]: ",

		// ---- question: which model provider
		"provider.question": "Which model provider will it call?",
		"provider.explain": "All three are wired up already — the agent code is the same either way,\n" +
			"and only the manifest and one pinned SDK change. The model id is pinned\n" +
			"rather than left as an alias, so an upgrade is something you decide.",
		"provider.prompt": "Choose 1–%d [1]: ",

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

		// ---- shared
		"common.notanoption": "  not one of the options",
		"common.givingup":    "giving up after three tries; nothing written",
	},

	"pt-BR": {
		"banner.title": "agentarch — vamos configurar seu projeto.",
		"banner.body": "Algumas perguntas em linguagem simples. Nada será escrito até você confirmar.\n" +
			"Pressione Enter para usar o padrão, ou q para sair.",

		"existing.question":     "Seu projeto já existe?",
		"existing.new":          "Novo — começar do zero com um projeto completo que funciona",
		"existing.adopt":        "Já existe — descrever o que há aqui e continuar a partir disso",
		"existing.refactor":     "Já existe — refatorar seguindo o padrão, com testes e revisão",
		"existing.hint.empty":   "este diretório parece vazio",
		"existing.hint.hascode": "já existe código aqui",
		"existing.prompt":       "Escolha 1–3 [%d]: ",

		"blueprint.question": "O que você quer construir?",
		"blueprint.runson":   "roda com %s",
		"blueprint.prompt":   "Escolha 1–%d (ou q para sair): ",

		"framework.question":   "Este blueprint traz código que roda para:",
		"framework.none":       "(o SDK do provedor direto)",
		"framework.label.none": "sem framework",
		"framework.prompt":     "Escolha 1–%d [1]: ",

		"provider.question": "Qual provedor de modelo ele vai chamar?",
		"provider.explain": "Os três já estão prontos — o código do agente é o mesmo em qualquer um,\n" +
			"e só mudam o manifesto e um SDK fixado. O id do modelo é fixado em vez de\n" +
			"ficar num alias, para que atualizar seja uma decisão sua.",
		"provider.prompt": "Escolha 1–%d [1]: ",

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

		"common.notanoption": "  não é uma das opções",
		"common.givingup":    "desisti após três tentativas; nada foi escrito",
	},
}
