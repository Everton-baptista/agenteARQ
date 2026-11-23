package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func recipeFS() fstest.MapFS {
	return fstest.MapFS{
		"verifiers/floor/install.sh":         {Data: []byte("#!/bin/sh\n")},
		"verifiers/floor/agentarch-secrets":  {Data: []byte("#!/bin/sh\n")},
		"verifiers/shared/check-links.mjs":   {Data: []byte("// links\n")},
		"verifiers/go-service/Dockerfile":    {Data: []byte("FROM x\nCOPY go.mod go.sum ./\nCOPY shared/ /opt/shared/\nCOPY --chown=0:0 tools.txt /opt/\nCOPY floor/ /opt/agentarch-floor-src/\n")},
		"verifiers/go-service/tools.txt":     {Data: []byte("pinned\n")},
		"verifiers/go-service/fixtures/a.go": {Data: []byte("package a\n")},
	}
}

func TestStageRecipeTakesRecipeFloorSharedAndProjectInputs(t *testing.T) {
	s, _ := fixture(t)
	write(t, s.Root, "go.mod", "module example.com/x\n\ngo 1.22\n")
	dir := t.TempDir()
	if err := s.stageRecipe(recipeFS(), "go-service", dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"Dockerfile", "go.mod", "go.sum", "tools.txt", "shared/check-links.mjs", "floor/install.sh", "floor/agentarch-secrets"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s not staged: %v", rel, err)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "floor/agentarch-secrets")); err != nil || info.Mode()&0o100 == 0 {
		t.Error("floor wrappers must stay executable")
	}
}

func TestStageRecipeExplainsAMissingInputAndRefusesSymlinks(t *testing.T) {
	s, _ := fixture(t)
	fsys := fstest.MapFS{"verifiers/python-api/Dockerfile": {Data: []byte("FROM x\nCOPY requirements-verifier.lock ./\n")}}
	err := s.stageRecipe(fsys, "python-api", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "pip-compile --generate-hashes") {
		t.Fatalf("missing lock not explained: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("host file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.Root, "requirements-verifier.lock")); err != nil {
		t.Fatal(err)
	}
	if err := s.stageRecipe(fsys, "python-api", t.TempDir()); err == nil {
		t.Fatal("a symlinked input pulled a host file into the build context")
	}
}

func TestProfileWithoutADefaultBaseExplainsTheChoice(t *testing.T) {
	s, _ := fixture(t) // typescript-web
	_, err := s.PrepareVerifier(context.Background(), PrepareOptions{Recipes: recipeFS()})
	if err == nil || !strings.Contains(err.Error(), "Playwright") || !strings.Contains(err.Error(), "Node") {
		t.Fatalf("expected the Node / Playwright choice to be explained, got %v", err)
	}
}

func TestLocalImageIDIsAcceptedAsAPinnedImage(t *testing.T) {
	r := &ContainerRunner{Image: "sha256:" + strings.Repeat("a", 64)}
	res := r.Run(context.Background(), t.TempDir(), Command{ID: "x", Program: "true"})
	if strings.Contains(res.Message, "No verifier image") {
		t.Fatalf("a content-addressed local image id was rejected: %s", res.Message)
	}
}
