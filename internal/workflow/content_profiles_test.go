package workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// The static-site and docs profiles run fixed wrappers installed in their images, without
// arguments the project could influence, followed by the security floor.
func TestContentProfilesRunTheirWrappersThenTheFloor(t *testing.T) {
	want := map[string][][2]string{
		"static-site": {{"html", "agentarch-html"}, {"lint", "agentarch-js-lint"}, {"links", "agentarch-site-links"}},
		"docs":        {{"markdown", "agentarch-markdown"}, {"links", "agentarch-docs-links"}},
	}
	for profile, checks := range want {
		cmds, err := ProfileCommands(profile)
		if err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		if len(cmds) != len(checks)+len(securityFloor) {
			t.Fatalf("%s: %d commands, want %d", profile, len(cmds), len(checks)+len(securityFloor))
		}
		for i, c := range checks {
			got := cmds[i]
			if got.ID != c[0] || got.Program != c[1] || len(got.Args) != 0 || got.Timeout <= 0 {
				t.Fatalf("%s check %d = %+v, want %s running %s with no arguments", profile, i, got, c[0], c[1])
			}
		}
		for i, f := range securityFloor {
			if got := cmds[len(checks)+i]; got.ID != f.ID || got.Program != f.Program {
				t.Fatalf("%s: floor check %s missing or out of order: %+v", profile, f.ID, got)
			}
		}
		if _, err := commandsFor(Product{Profile: profile, Checks: []CheckSpec{{ID: "skip", Program: "true", TimeoutSeconds: 1}}}); err == nil {
			t.Fatalf("%s accepted custom checks; built-in profiles cannot be weakened", profile)
		}
	}
}

func TestContentProfileFailureBlocksVerification(t *testing.T) {
	for profile, ids := range map[string][]string{"static-site": {"html", "lint", "links"}, "docs": {"markdown", "links"}} {
		root := repo(t)
		v, err := verifyProfile(context.Background(), root, profile, &fakeRunner{})
		if err != nil || !v.Passed || len(v.Checks) != len(ids)+len(securityFloor) {
			t.Fatalf("%s clean verification = %+v, %v", profile, v, err)
		}
		for _, id := range ids {
			for _, status := range []string{"failed", "inconclusive"} {
				v, err := verifyProfile(context.Background(), root, profile, &fakeRunner{statusFor: map[string]string{id: status}})
				if err != nil || v.Passed {
					t.Fatalf("%s %s %s: passed=%v err=%v", profile, id, status, v.Passed, err)
				}
			}
		}
	}
}

// Profiles(), the profile table and both JSON schemas enumerate the same profiles.
func TestProfileEnumerationsAgree(t *testing.T) {
	listed := append([]string{}, Profiles()...)
	sort.Strings(listed)
	defined := make([]string, 0, len(profiles))
	for name := range profiles {
		defined = append(defined, name)
	}
	sort.Strings(defined)
	if !reflect.DeepEqual(listed, defined) {
		t.Fatalf("Profiles() = %v, profile table = %v", listed, defined)
	}
	for _, schema := range []struct {
		file string
		path []string
	}{
		{"product.schema.json", []string{"properties", "profile", "enum"}},
		{"workflow.state.schema.json", []string{"$defs", "fingerprint", "properties", "profile", "enum"}},
	} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "schemas", schema.file))
		if err != nil {
			t.Fatal(err)
		}
		var node any
		if err := json.Unmarshal(raw, &node); err != nil {
			t.Fatalf("%s: %v", schema.file, err)
		}
		enum := findEnum(node, schema.path)
		if enum == nil {
			t.Fatalf("%s: no profile enum at %v", schema.file, schema.path)
		}
		sort.Strings(enum)
		if !reflect.DeepEqual(enum, listed) {
			t.Fatalf("%s profile enum = %v, want %v", schema.file, enum, listed)
		}
	}
}

func findEnum(node any, path []string) []string {
	for _, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		node = m[key]
	}
	items, ok := node.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}
