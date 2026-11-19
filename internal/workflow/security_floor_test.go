package workflow

import (
	"context"
	"strings"
	"testing"
)

// Every profile carries the floor, and nothing in product.yaml can remove or redefine it.
func TestSecurityFloorIsInEveryProfileAndCannotBeRedefined(t *testing.T) {
	products := []Product{
		{Profile: "typescript-web"},
		{Profile: "python-api"},
		{Profile: "go-service"},
		{Profile: "static-site"},
		{Profile: "docs"},
		{Profile: "universal", Checks: []CheckSpec{{ID: "unit", Program: "cargo", Args: []string{"test"}, TimeoutSeconds: 60}}},
	}
	for _, p := range products {
		cmds, err := commandsFor(p)
		if err != nil {
			t.Fatalf("%s: %v", p.Profile, err)
		}
		got := map[string]Command{}
		for _, c := range cmds {
			got[c.ID] = c
		}
		for _, f := range securityFloor {
			c, ok := got[f.ID]
			if !ok || c.Program != f.Program || len(c.Args) != 0 {
				t.Fatalf("%s: floor check %s missing or altered: %+v", p.Profile, f.ID, c)
			}
		}
	}
	for _, id := range FloorCheckIDs() {
		_, err := commandsFor(Product{Profile: "universal", Checks: []CheckSpec{{ID: id, Program: "true", TimeoutSeconds: 1}}})
		if err == nil || !strings.Contains(err.Error(), "reserved for the security floor") {
			t.Fatalf("redefining %s: %v", id, err)
		}
	}
	if _, err := commandsFor(Product{Profile: "universal"}); err == nil {
		t.Fatal("universal with only the floor passed; the product itself must declare checks")
	}
}

// A failed or inconclusive floor check blocks readiness like any other check.
func TestFloorFailureBlocksVerification(t *testing.T) {
	for _, id := range FloorCheckIDs() {
		for _, status := range []string{"failed", "inconclusive"} {
			root := repo(t)
			v, err := verifyProfile(context.Background(), root, "python-api", &fakeRunner{statusFor: map[string]string{id: status}})
			if err != nil || v.Passed {
				t.Fatalf("%s %s: passed=%v err=%v", id, status, v.Passed, err)
			}
		}
	}
}
