package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportSpecReadsKiroSpecKitAndOpenSpec(t *testing.T) {
	s := riskTree(t, map[string]string{
		".kiro/specs/cart/requirements.md": "# Requirements\n\n### Requirement 1\n\n1. WHEN the user adds an item THEN the system SHALL show the new total\n2. Narrative text without a requirement.\n",
		"specs/001-login/spec.md":          "## Requirements\n\n- **FR-001**: System MUST lock an account after 5 failed attempts\n\n```\nMUST not be read from a code block\n```\n\n1. **Given** a locked account, **When** the user logs in, **Then** an unlock email is offered\n",
		"openspec/specs/search/spec.md":    "### Requirement: Search\nThe system SHALL return results in under 1 second.\n#### Scenario: empty query\n- WHEN the query is empty THEN no request is sent\n",
		documents[1]:                       "# Specification\n\n- AC-1: TODO\n",
	})
	r, err := s.ImportSpec("auto")
	if err != nil {
		t.Fatal(err)
	}
	if r.Written != documents[1] || r.Criteria != 5 || len(r.Sources) != 3 {
		t.Fatalf("unexpected result: %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, documents[1]))
	got := string(b)
	for _, want := range []string{
		"AC-1: WHEN the user adds an item THEN the system SHALL show the new total (from .kiro/specs/cart/requirements.md:5)",
		"System MUST lock an account after 5 failed attempts",
		"Given a locked account, When the user logs in, Then an unlock email is offered",
		"The system SHALL return results in under 1 second.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "code block") || strings.Contains(got, "Narrative") {
		t.Errorf("imported text that is not a requirement:\n%s", got)
	}
}

func TestImportSpecNeverOverwritesAFilledSpec(t *testing.T) {
	s := riskTree(t, map[string]string{
		".kiro/specs/x/requirements.md": "1. WHEN a THEN the system SHALL b\n",
		documents[1]:                    "# Specification\n\n- AC-1: Visitors can book a class.\n",
	})
	r, err := s.ImportSpec("kiro")
	if err != nil {
		t.Fatal(err)
	}
	if r.Written != ImportedSpecPath {
		t.Fatalf("written to %s", r.Written)
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, documents[1]))
	if !strings.Contains(string(b), "Visitors can book a class") {
		t.Fatal("the owner's spec was overwritten")
	}
}

func TestImportSpecSaysWhatIsMissing(t *testing.T) {
	s := riskTree(t, map[string]string{"README.md": "x"})
	if _, err := s.ImportSpec("auto"); err == nil || !strings.Contains(err.Error(), "Kiro") {
		t.Fatalf("no source: %v", err)
	}
	if _, err := s.ImportSpec("cucumber"); err == nil {
		t.Fatal("unknown format accepted")
	}
}
