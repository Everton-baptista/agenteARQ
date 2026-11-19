package workflow

import (
	"os/exec"
	"strings"
	"testing"
)

// A clone has the committed product.yaml but no private state. Init must adopt it without
// the facts being typed again, and must refuse a value that contradicts it.
func TestInitAdoptsCommittedProductWithoutFlags(t *testing.T) {
	origin := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", origin).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	s, err := New(origin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Init(InitOptions{Name: "Shop", Profile: "python-api", Goal: "Sell", Author: "ana"}); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	run(origin, "add", "-A")
	run(origin, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-qm", "product")
	clone := t.TempDir()
	run(clone, "clone", "-q", origin, ".")

	c, err := New(clone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Init(InitOptions{Name: "Other"}); err == nil || !strings.Contains(err.Error(), "omit --name") {
		t.Fatalf("conflicting name: %v", err)
	}
	if _, err = c.Init(InitOptions{Profile: "typescript-web"}); err == nil || !strings.Contains(err.Error(), "omit --profile") {
		t.Fatalf("conflicting profile: %v", err)
	}
	snap, err := c.Init(InitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Product.Name != "Shop" || snap.Product.Profile != "python-api" || snap.State.Phase != "draft" {
		t.Fatalf("adopted %+v in phase %s", snap.Product, snap.State.Phase)
	}
}
