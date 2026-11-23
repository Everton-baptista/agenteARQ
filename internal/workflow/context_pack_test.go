package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNextTaskCarriesABoundedContextPack(t *testing.T) {
	s := specifiedFixture(t)
	write(t, s.Root, ScenariosPath, `schema_version: '1.0'
scenarios:
  - {id: own-orders, acceptance: [AC-1], given: a customer with orders, when: they search, then: their orders are listed, kind: happy}
  - {id: other-tenant, acceptance: [AC-2], given: two tenants, when: A asks for B's order, then: not found is shown, kind: permission}
`)
	for _, phase := range []string{"planned", "implementing"} {
		if _, e := s.Checkpoint(CheckpointRequest{Phase: phase}); e != nil {
			t.Fatal(e)
		}
	}
	n, e := s.Next()
	if e != nil {
		t.Fatal(e)
	}
	c := n.Context
	if c == nil || c.TaskID != "order-search" {
		t.Fatalf("no context pack for the ready task: %+v", n)
	}
	if len(c.Criteria) != 2 || !strings.Contains(c.Criteria[1], "another tenant") || len(c.Scenarios) != 2 || c.Checks[0] != "test" {
		t.Fatalf("pack is missing the task's contract: %+v", c)
	}
	b, _ := json.Marshal(c)
	if len(b) > ContextPackLimit+512 {
		t.Fatalf("pack is %d bytes", len(b))
	}
}

func TestContextPackTruncatesLongCriteria(t *testing.T) {
	s, _ := fixture(t)
	long := strings.Repeat("The customer sees a very detailed order history ", 400)
	write(t, s.Root, documents[1], "# Spec\n\nAC-1: "+long+"\n")
	pack := s.taskContext(Task{ID: "t", Title: "Long task", Acceptance: []string{"AC-1"}, Checks: []string{"test"}})
	b, _ := json.Marshal(pack)
	if !pack.Truncated || len(b) > ContextPackLimit+512 || !strings.Contains(pack.Criteria[0], "truncated") {
		t.Fatalf("not truncated within bounds: %d bytes, truncated=%v", len(b), pack.Truncated)
	}
}

func TestCriterionTextPrefersTheListItem(t *testing.T) {
	spec := "# Spec\n\nIntro paragraph.\n\n- AC-1: first\n- AC-10: tenth\n\nAC-2 is described\nacross two lines.\n"
	if got := criterionText(spec, "AC-1"); got != "- AC-1: first" {
		t.Errorf("AC-1 = %q", got)
	}
	if got := criterionText(spec, "AC-2"); got != "AC-2 is described\nacross two lines." {
		t.Errorf("AC-2 = %q", got)
	}
	if got := criterionText(spec, "AC-7"); got != "AC-7" {
		t.Errorf("missing = %q", got)
	}
}
