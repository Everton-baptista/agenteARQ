package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// refusalSources are the workflow files whose errors a product owner meets at a gate.
// Infrastructure errors (locks, the state store, container plumbing) are not refusals.
var refusalSources = []string{"workflow.go", "release.go", "pr.go", "decision.go", "verify.go", "acceptance.go"}

var refusalLiteral = regexp.MustCompile(`(?:errors\.New|fmt\.Errorf)\(("(?:[^"\\]|\\.)*")`)

// TestEveryWorkflowRefusalIsExplained fails when a gate can say no without telling a
// non-technical owner what it means and what to do next.
func TestEveryWorkflowRefusalIsExplained(t *testing.T) {
	for _, name := range refusalSources {
		path := filepath.Join("..", "..", "internal", "workflow", name)
		src, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range refusalLiteral.FindAllStringSubmatch(string(src), -1) {
			msg, err := strconv.Unquote(m[1])
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			// Format verbs stand for values; replace them with a plausible one.
			msg = regexp.MustCompile(`%[qsdvw]`).ReplaceAllString(msg, "x")
			if strings.HasPrefix(msg, "x: ") || msg == "x" {
				continue // wraps another error that carries its own message
			}
			if _, ok := lookupRefusal(msg); !ok {
				t.Errorf("%s: refusal %q has no plain-language entry in refusals.go", name, msg)
			}
		}
	}
}

func TestRefusalEntriesAreCompleteInEveryLanguage(t *testing.T) {
	seen := map[string]bool{}
	for _, x := range refusals {
		if seen[x.ID] {
			t.Errorf("duplicate refusal id %s", x.ID)
		}
		seen[x.ID] = true
		if !strings.HasPrefix(x.ID, "workflow.") {
			t.Errorf("%s: refusal ids start with workflow.", x.ID)
		}
		for _, l := range languages {
			text := x.Text[l.Code]
			if strings.TrimSpace(text[0]) == "" || strings.TrimSpace(text[1]) == "" {
				t.Errorf("%s: missing %s meaning or next step", x.ID, l.Code)
			}
		}
	}
}

func TestRefusalExplanationIsAddedNotSubstituted(t *testing.T) {
	defer func(prev string) { lang = prev }(lang)
	lang = "pt-BR"
	var b bytes.Buffer
	explainRefusal(&b, errors.New("cannot skip workflow phases"))
	out := b.String()
	if !strings.Contains(out, "O que significa:") || !strings.Contains(out, "agentarch explain workflow.cannot_skip") {
		t.Fatalf("explanation missing: %q", out)
	}
	b.Reset()
	explainRefusal(&b, errors.New("some infrastructure failure"))
	if b.Len() != 0 {
		t.Fatalf("an unknown error got an invented explanation: %q", b.String())
	}
}
