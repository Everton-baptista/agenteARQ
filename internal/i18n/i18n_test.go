package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func fsWith(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func withHeader(source, sha, body string) string {
	return "---\nlang: pt-BR\nsource: " + source + "\nsource_sha256: \"" + sha + "\"\n---\n\n" + body
}

func TestParseFrontMatterSplitsHeaderFromBody(t *testing.T) {
	fm, body, ok := ParseFrontMatter(withHeader("content/core/en/00.md", strings.Repeat("a", 64), "# Título\n"))
	if !ok {
		t.Fatal("header not recognised")
	}
	if fm.Lang != "pt-BR" {
		t.Errorf("lang = %q", fm.Lang)
	}
	if strings.Contains(body, "source_sha256") {
		t.Error("header leaked into the body; it would spend the assistant's context budget")
	}
	if !strings.HasPrefix(body, "# Título") {
		t.Errorf("body = %q", body)
	}
}

func TestFileWithoutHeaderIsPassedThrough(t *testing.T) {
	if _, _, ok := ParseFrontMatter("# Just a heading\n"); ok {
		t.Error("a file with no front matter must not be reported as having one")
	}
}

func TestUpToDateTranslationIsClean(t *testing.T) {
	src := "# Rules\n"
	fsys := fsWith(map[string]string{
		"core/en/00-identity.md":    src,
		"core/pt-BR/00-identity.md": withHeader("content/core/en/00-identity.md", Digest([]byte(src)), "# Regras\n"),
	})
	got, err := Check(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if p := Problems(got); len(p) != 0 {
		t.Fatalf("expected no problems, got %v", p)
	}
}

// The whole reason the header exists. A stale translation answers the reader's question with
// authority, using a rule that has since changed — worse than no translation, which would at
// least send them to the source.
func TestStaleTranslationIsReported(t *testing.T) {
	fsys := fsWith(map[string]string{
		"core/en/00-identity.md":    "# Rules, revised\n",
		"core/pt-BR/00-identity.md": withHeader("content/core/en/00-identity.md", Digest([]byte("# Rules\n")), "# Regras\n"),
	})
	got, _ := Check(fsys)
	p := Problems(got)
	if len(p) != 1 || !p[0].Stale {
		t.Fatalf("a stale translation must be reported, got %v", p)
	}
	if !strings.Contains(p[0].String(), "AA-I18N-016") {
		t.Error("the finding should carry its stable id")
	}
}

func TestTranslationWithNoHeaderIsReported(t *testing.T) {
	fsys := fsWith(map[string]string{
		"core/en/00-identity.md":    "# Rules\n",
		"core/pt-BR/00-identity.md": "# Regras\n",
	})
	p := Problems(mustCheck(t, fsys))
	if len(p) != 1 || !p[0].NoHeader {
		t.Fatalf("a translation with no header must be reported, got %v", p)
	}
}

func TestTranslationPointingAtAMissingSourceIsReported(t *testing.T) {
	fsys := fsWith(map[string]string{
		"core/en/00-identity.md":   "# Rules\n",
		"core/pt-BR/99-removed.md": withHeader("content/core/en/99-removed.md", strings.Repeat("b", 64), "# Removido\n"),
	})
	p := Problems(mustCheck(t, fsys))
	if len(p) != 1 || !p[0].Missing {
		t.Fatalf("a translation whose source was removed must be reported, got %v", p)
	}
}

// English is normative and derives from nothing, so it is never checked against itself.
func TestSourceLanguageIsNotChecked(t *testing.T) {
	fsys := fsWith(map[string]string{"core/en/00-identity.md": "# Rules\n"})
	if got := mustCheck(t, fsys); len(got) != 0 {
		t.Fatalf("the source language must not be checked, got %v", got)
	}
}

func mustCheck(t *testing.T, fsys fstest.MapFS) []Status {
	t.Helper()
	got, err := Check(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
