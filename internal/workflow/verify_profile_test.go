package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileMeasurementDoesNotInitializeWorkflow(t *testing.T) {
	root := repo(t)
	v, err := verifyProfile(context.Background(), root, "python-api", &fakeRunner{})
	if err != nil || !v.Passed || len(v.Checks) != 5 {
		t.Fatalf("verification = %+v, %v", v, err)
	}
	for _, rel := range []string{"agentarch", ".git/agentarch-workflow"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Fatalf("measurement wrote %s: %v", rel, err)
		}
	}
}

func TestProfileMeasurementCannotPassStaleOrMissingChecks(t *testing.T) {
	root := repo(t)
	v, err := verifyProfile(context.Background(), root, "typescript-web", &fakeRunner{after: func() { write(t, root, "new.ts", "changed") }})
	if err != nil || v.Passed || v.Checks[len(v.Checks)-1].ID != "source-stability" {
		t.Fatalf("stale verification = %+v, %v", v, err)
	}
	for _, status := range []string{"failed", "inconclusive"} {
		v, err := verifyProfile(context.Background(), root, "python-api", &fakeRunner{status: status})
		if err != nil || v.Passed {
			t.Fatalf("%s verification = %+v, %v", status, v, err)
		}
	}
	if _, err := verifyProfile(context.Background(), root, "universal", &fakeRunner{}); err == nil {
		t.Fatal("empty checks accepted")
	}
	if _, err := VerifyProfile(context.Background(), root, "python-api", "untrusted:latest"); err == nil {
		t.Fatal("floating image accepted")
	}
}
