package workflow

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// specifiedFixture brings a product to specified with spec.md naming AC-1 and AC-2, an ADR and a
// task, so the next checkpoint is planned.
func specifiedFixture(t *testing.T) *Service {
	t.Helper()
	s, _ := fixture(t)
	write(t, s.Root, documents[0], "# Brief\n\nCustomers need to find their own orders quickly.\n")
	write(t, s.Root, documents[1], "# Spec\n\n- AC-1: Customer can find only their own orders.\n- AC-2: Searching another tenant's order returns not found.\n")
	write(t, s.Root, documents[2], testADR)
	td := TaskDocument{SchemaVersion: SchemaVersion, Tasks: []Task{{ID: "order-search", Title: "Implement tenant-scoped order search", Acceptance: []string{"AC-1", "AC-2"}, Checks: []string{"test"}}}}
	b, _ := yaml.Marshal(td)
	write(t, s.Root, TasksPath, string(b))
	if _, e := s.Checkpoint(CheckpointRequest{Phase: "specified"}); e != nil {
		t.Fatal(e)
	}
	return s
}

func hasAdvice(n NextAction, want string) bool {
	for _, a := range n.Advice {
		if a == want {
			return true
		}
	}
	return false
}

func TestScenariosAbsentIsAdviceNotABlock(t *testing.T) {
	s := specifiedFixture(t)
	n, e := s.Next()
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{AdviceScenarios, AdvicePersonas, AdviceDesign} {
		if !hasAdvice(n, want) {
			t.Errorf("advice missing %q: %v", want, n.Advice)
		}
	}
	if _, e := s.Checkpoint(CheckpointRequest{Phase: "planned"}); e != nil {
		t.Fatalf("absent scenarios must not block planned: %v", e)
	}
}

func TestScenariosPresentMustCoverEveryCriterion(t *testing.T) {
	cases := []struct {
		name, body string
		wantGap    []string
	}{
		{"uncovered criterion", `schema_version: '1.0'
scenarios:
  - id: own-orders
    acceptance: [AC-1]
    given: a signed-in customer with two orders
    when: they search for their orders
    then: both orders are listed
    kind: happy
`, []string{"AC-2 has no scenario"}},
		{"unknown criterion and empty then", `schema_version: '1.0'
scenarios:
  - id: other-tenant
    acceptance: [AC-2, AC-9]
    given: two tenants
    when: tenant A asks for B's order
    then: ""
    kind: permission
  - id: own-orders
    acceptance: [AC-1]
    given: a customer
    when: they search
    then: TODO
    kind: sideways
`, []string{"references missing acceptance criterion AC-9", "other-tenant needs a concrete then", "own-orders needs a concrete then", "own-orders kind must be one of"}},
		{"duplicate ids", `schema_version: '1.0'
scenarios:
  - {id: a, acceptance: [AC-1, AC-2], given: g, when: w, then: t, kind: happy}
  - {id: a, acceptance: [AC-1], given: g, when: w, then: t, kind: edge}
`, []string{"duplicate scenario id a"}},
		{"unknown field is a schema error", `schema_version: '1.0'
scenarios:
  - {id: a, acceptance: [AC-1, AC-2], given: g, when: w, then: t, kind: happy, priority: high}
`, []string{"field priority not found"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := specifiedFixture(t)
			write(t, s.Root, ScenariosPath, c.body)
			_, e := s.Checkpoint(CheckpointRequest{Phase: "planned"})
			if e == nil {
				t.Fatal("malformed scenarios advanced to planned")
			}
			for _, w := range c.wantGap {
				if !strings.Contains(e.Error(), w) {
					t.Errorf("error %q does not mention %q", e, w)
				}
			}
		})
	}
}

func TestCompleteScenariosAndPersonasAdvance(t *testing.T) {
	s := specifiedFixture(t)
	write(t, s.Root, ScenariosPath, `schema_version: '1.0'
scenarios:
  - {id: own-orders, acceptance: [AC-1], given: a customer with orders, when: they search, then: their orders are listed, kind: happy}
  - {id: other-tenant, acceptance: [AC-2], given: two tenants, when: A asks for B's order, then: not found is shown, kind: permission}
`)
	write(t, s.Root, PersonasPath, `schema_version: '1.0'
personas:
  - {id: shop-owner, goal: Find yesterday's orders before opening the shop, familiarity: none, device: phone, accessibility_needs: [large text]}
  - {id: snooper, goal: Read another tenant's orders by guessing ids, familiarity: expert, device: laptop, adversarial: true}
`)
	n, _ := s.Next()
	if hasAdvice(n, AdviceScenarios) || hasAdvice(n, AdvicePersonas) {
		t.Fatalf("advice repeated for files that exist: %v", n.Advice)
	}
	if _, e := s.Checkpoint(CheckpointRequest{Phase: "planned"}); e != nil {
		t.Fatal(e)
	}
}

func TestMalformedPersonasBlockPlanned(t *testing.T) {
	s := specifiedFixture(t)
	write(t, s.Root, PersonasPath, `schema_version: '1.0'
personas:
  - {id: Shop Owner, goal: short, familiarity: guru, device: ""}
`)
	_, e := s.Checkpoint(CheckpointRequest{Phase: "planned"})
	if e == nil {
		t.Fatal("malformed personas advanced")
	}
	for _, w := range []string{"needs a lowercase id", "needs a concrete goal", "familiarity must be one of", "needs a device"} {
		if !strings.Contains(e.Error(), w) {
			t.Errorf("%q not in %v", w, e)
		}
	}
}

func TestSpecCriteriaInOrder(t *testing.T) {
	got := specCriteria([]byte("AC-2 then AC-1, again AC-2 and BAC-3 and AC-10"))
	if strings.Join(got, ",") != "AC-2,AC-1,AC-10" {
		t.Fatalf("got %v", got)
	}
}
