package workflow

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceValidationFailureIsNotInfrastructureFailure(t *testing.T) {
	root := repo(t)
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the pytest artifact: an absolute symlink, even to another file
	// under this source tree, is not portable into an isolated source snapshot.
	if err = os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "pytest-current")); err != nil {
		t.Skip(err)
	}
	err = s.copySource(t.TempDir())
	var invalid *SourceValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("escaping symlink must be a typed source violation: %v", err)
	}
	initial := CheckResult{ID: "test", Status: "inconclusive", ExitCode: -1}
	if got := sourceStagingFailure(initial, err); got.Status != "failed" || got.ExitCode != -1 {
		t.Fatalf("unexecuted invalid source must fail: %+v", got)
	}
	if err = os.Remove(filepath.Join(root, "pytest-current")); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	write(t, dest, "README.md", "existing staging file")
	err = s.copySource(dest)
	if !errors.Is(err, os.ErrExist) || errors.As(err, &invalid) {
		t.Fatalf("real filesystem failure was relabeled: %v", err)
	}
	if got := sourceStagingFailure(initial, err); got.Status != "inconclusive" {
		t.Fatalf("staging I/O must stay inconclusive: %+v", got)
	}
}

func TestSourcePathErrorsKeepTheirTypeAndIOCause(t *testing.T) {
	root := t.TempDir()
	var invalid *SourceValidationError
	if _, err := safePath(root, "../outside"); !errors.As(err, &invalid) {
		t.Fatalf("path escape: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "redirect")); err != nil {
		t.Skip(err)
	}
	if _, err := safePath(root, "redirect/file"); !errors.As(err, &invalid) {
		t.Fatalf("symlink parent: %v", err)
	}
	write(t, root, "regular", "a file is not a directory")
	_, err := safePath(root, "regular/child")
	var pathError *os.PathError
	if !errors.As(err, &pathError) || errors.As(err, &invalid) {
		t.Fatalf("Lstat I/O cause must not be relabeled: %v", err)
	}
}

func TestVerifyProfileUnsafeSourceFailsBeforeAnyContainerExecution(t *testing.T) {
	root := repo(t)
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "pytest-current")); err != nil {
		t.Skip(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err = os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	// A local sentinel executable detects accidental invocation; no Docker/model runs.
	marker := filepath.Join(bin, "invoked")
	if err = os.WriteFile(filepath.Join(bin, "podman"), []byte("#!/bin/sh\n: > '"+marker+"'\nexit 125\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	v, err := VerifyProfile(context.Background(), root, "python-api", "fixture@sha256:"+strings.Repeat("a", 64))
	if err != nil || v.Passed || len(v.Checks) != 5 {
		t.Fatalf("unsafe candidate verification: %+v, %v", v, err)
	}
	for _, check := range v.Checks {
		if check.Status != "failed" || !strings.Contains(check.Message, "symlink") {
			t.Fatalf("source violation misclassified: %+v", check)
		}
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("container was invoked for invalid source: %v", err)
	}
}
