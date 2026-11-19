package main

import (
	"bufio"
	"strings"
	"testing"
)

// A key present in one language and absent from another prints English in the middle of a
// Portuguese interview. The fallback exists so that degrades to readable rather than to blank —
// but a fallback nobody is told about is a translation that quietly never gets finished.
func TestEveryKeyExistsInEveryLanguage(t *testing.T) {
	source, ok := catalog[SourceLang]
	if !ok {
		t.Fatalf("no catalogue for the source language %q", SourceLang)
	}
	if len(source) == 0 {
		t.Fatal("the source catalogue is empty")
	}

	for _, code := range languageCodes() {
		entries, ok := catalog[code]
		if !ok {
			t.Errorf("%q is offered in the first question and has no catalogue", code)
			continue
		}
		for key := range source {
			if _, ok := entries[key]; !ok {
				t.Errorf("%s is missing key %q", code, key)
			}
		}
		for key := range entries {
			if _, ok := source[key]; !ok {
				t.Errorf("%s has key %q, which does not exist in %s — a translation of "+
					"something nobody says", code, key, SourceLang)
			}
		}
	}
}

// A format string that gains or loses a verb between languages prints the wrong value, or panics
// with %!s(MISSING). Counting them is crude and catches exactly that.
func TestFormatVerbsMatchAcrossLanguages(t *testing.T) {
	count := func(s string) int {
		n := 0
		for i := 0; i < len(s)-1; i++ {
			if s[i] != '%' {
				continue
			}
			if s[i+1] == '%' {
				i++ // an escaped percent is not a verb
				continue
			}
			n++
		}
		return n
	}

	for key, src := range catalog[SourceLang] {
		want := count(src)
		for _, code := range languageCodes() {
			if code == SourceLang {
				continue
			}
			got, ok := catalog[code][key]
			if !ok {
				continue // already reported by the parity test
			}
			if n := count(got); n != want {
				t.Errorf("%s %q has %d format verb(s), %s has %d:\n  %s\n  %s",
					code, key, n, SourceLang, want, src, got)
			}
		}
	}
}

// Nothing in the catalogue may be empty. A blank translation is worse than a missing one: the
// fallback never fires, and the interview prints nothing where a question should be.
func TestNoCatalogueEntryIsBlank(t *testing.T) {
	for code, entries := range catalog {
		for key, s := range entries {
			if strings.TrimSpace(s) == "" {
				t.Errorf("%s %q is blank", code, key)
			}
		}
	}
}

func TestLookupFallsBackToEnglish(t *testing.T) {
	prev := lang
	defer func() { lang = prev }()

	lang = "pt-BR"
	if got := t_("banner.title"); got != catalog["pt-BR"]["banner.title"] {
		t.Errorf("chose pt-BR and got %q", got)
	}

	// A language with no catalogue at all must not blank the interview.
	lang = "xx"
	if got := t_("banner.title"); got != catalog["en"]["banner.title"] {
		t.Errorf("unknown language should fall back to English, got %q", got)
	}

	// A key nobody defined is loud rather than silent.
	if got := t_("no.such.key"); got != "[no.such.key]" {
		t.Errorf("an undefined key returned %q; it should be visibly wrong", got)
	}
}

// t_ is t, renamed inside this file only because `t` is also the testing parameter.
func t_(key string) string { return t(key) }

// The languages the interview offers must be the languages the content tree can render, or
// somebody picks Portuguese and gets English instruction files.
func TestOfferedLanguagesHaveContent(t *testing.T) {
	for _, code := range languageCodes() {
		if !knownLanguage(code) {
			t.Errorf("%q is in the catalogue and not offered", code)
		}
	}
	// pt-BR and en are what content/core/ and content/standards/ ship. Offering a third
	// without translating the core would render an English core under a Portuguese question.
	want := map[string]bool{"en": true, "pt-BR": true}
	for _, code := range languageCodes() {
		if !want[code] {
			t.Errorf("%q is offered but content/core/%s does not exist — check before adding",
				code, code)
		}
	}
}

// withAnswers drives the interview from a script instead of a keyboard.
//
// `ask` reads a package-level reader, so swapping it makes the whole interview testable — which
// it was not, and the gap mattered: the questions are the part of this tool most people see and
// the part with the least coverage. A pty is not a substitute; whether a test process has a
// controlling terminal varies between a laptop, CI and a container.
func withAnswers(t *testing.T, script string, fn func()) {
	t.Helper()
	prevIn, prevLang := stdin, lang
	stdin = bufio.NewReader(strings.NewReader(script))
	defer func() { stdin, lang = prevIn, prevLang }()
	fn()
}

// The first question is asked in both languages, because there is no chosen language yet in
// which to ask it.
func TestLanguageQuestionIsAskedInBothLanguages(t *testing.T) {
	var out string
	withAnswers(t, "1\n", func() {
		out, _, _ = capture(t, func() int {
			askLanguage()
			return exitOK
		})
	})
	for _, want := range []string{"English", "Português", "Escolha", "Choose"} {
		if !strings.Contains(out, want) {
			t.Errorf("the language question does not contain %q:\n%s", want, out)
		}
	}
}

func TestLanguageAnswersAreAccepted(t *testing.T) {
	for _, tc := range []struct {
		answer string
		want   string
	}{
		{"1\n", "en"},
		{"2\n", "pt-BR"},
		{"\n", "en"},           // Enter takes the default
		{"pt\n", "pt-BR"},      // the shorthand somebody actually types
		{"pt-BR\n", "pt-BR"},   // the code itself
		{"portugu\n", "pt-BR"}, // a prefix of the name
		{"english\n", "en"},
	} {
		t.Run(strings.TrimSpace(tc.answer), func(t *testing.T) {
			var got string
			withAnswers(t, tc.answer, func() {
				capture(t, func() int {
					code, quit := askLanguage()
					if quit {
						t.Error("answering should not quit")
					}
					got = code
					return exitOK
				})
			})
			if got != tc.want {
				t.Errorf("answered %q and got %q, want %q",
					strings.TrimSpace(tc.answer), got, tc.want)
			}
		})
	}
}

// Choosing Portuguese must translate every question, not the first one. A menu that switches
// language halfway reads as broken content rather than as a missing translation.
func TestChoosingPortugueseTranslatesTheWholeInterview(t *testing.T) {
	prev := lang
	defer func() { lang = prev }()
	lang = "pt-BR"

	stdout, _, _ := capture(t, func() int {
		withAnswers(t, "1\n", func() { askExisting(projectState{}) })
		withAnswers(t, "1\n", func() { askJurisdictions() })
		return exitOK
	})

	// Every question, and the hint under the first one.
	for _, want := range []string{
		"Seu projeto já existe?",
		"Onde seu projeto vai ser usado?",
		"este diretório parece vazio",
		"Brasil",
		"LGPD",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("Portuguese interview is missing %q:\n%s", want, stdout)
		}
	}
	// And nothing left in English.
	for _, unwanted := range []string{"Is this a new agent", "Where are the people", "Brazil "} {
		if strings.Contains(stdout, unwanted) {
			t.Errorf("English leaked into the Portuguese interview: %q", unwanted)
		}
	}
}

// The third option is what "refactor" answers. It shares the adoption path — both describe what
// is already here — and differs in what gets installed afterwards.
func TestThirdOptionAsksForRefactoring(t *testing.T) {
	var adopt, refactor bool
	withAnswers(t, "3\n", func() {
		capture(t, func() int {
			adopt, refactor, _, _ = askExisting(projectState{HasCode: true})
			return exitOK
		})
	})
	if !adopt {
		t.Error("refactoring still has to adopt what is there first")
	}
	if !refactor {
		t.Error("option 3 did not select the refactoring path")
	}

	// And option 2 must not.
	withAnswers(t, "2\n", func() {
		capture(t, func() int {
			adopt, refactor, _, _ = askExisting(projectState{HasCode: true})
			return exitOK
		})
	})
	if !adopt || refactor {
		t.Errorf("option 2 gave adopt=%v refactor=%v, want true/false", adopt, refactor)
	}
}
