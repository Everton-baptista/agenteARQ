package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func lockOf(t *testing.T, dir string) *Lock {
	t.Helper()
	l, err := Build(dir, "1.0.0", "2026-07-28")
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestUntouchedTreeHasNoChanges(t *testing.T) {
	dir := tree(t, map[string]string{
		"core/en/10-invariants.md":   "rules\n",
		"packs/core.agent/pack.yaml": "pack: {}\n",
	})
	l := lockOf(t, dir)

	changes, err := Diff(dir, l)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}
}

func TestLockDoesNotRecordItself(t *testing.T) {
	dir := tree(t, map[string]string{"core/en/00.md": "x\n"})
	l := lockOf(t, dir)
	if _, present := l.Files[Name]; present {
		t.Fatal("the lock must not record its own hash; it changes every time it is written")
	}
}

func TestEditIsDetected(t *testing.T) {
	dir := tree(t, map[string]string{"core/en/10-invariants.md": "rules\n"})
	l := lockOf(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "core/en/10-invariants.md"),
		[]byte("rules\n# my own addition\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ := Diff(dir, l)
	if len(changes) != 1 || changes[0].Kind != "edited" {
		t.Fatalf("expected one edit, got %v", changes)
	}
}

func TestDeletionIsDetected(t *testing.T) {
	dir := tree(t, map[string]string{"core/en/a.md": "x\n", "core/en/b.md": "y\n"})
	l := lockOf(t, dir)

	if err := os.Remove(filepath.Join(dir, "core/en/b.md")); err != nil {
		t.Fatal(err)
	}
	changes, _ := Diff(dir, l)
	if len(changes) != 1 || changes[0].Kind != "deleted" {
		t.Fatalf("expected one deletion, got %v", changes)
	}
}

// An added file survives the upgrade, so it is reported as a distinct kind rather than as an
// edit — the consequences are different and conflating them would misinform the reader.
func TestAdditionIsDistinctFromEdit(t *testing.T) {
	dir := tree(t, map[string]string{"core/en/a.md": "x\n"})
	l := lockOf(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "core/en/mine.md"), []byte("z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, _ := Diff(dir, l)
	if len(changes) != 1 || changes[0].Kind != "added" {
		t.Fatalf("expected one addition, got %v", changes)
	}
}

// The reason this package exists. Comparing an installed tree against a newer payload cannot
// tell "the project edited this" from "the standard changed this" — every upstream improvement
// would look like a local edit and block its own upgrade.
func TestUpstreamChangeIsNotMistakenForALocalEdit(t *testing.T) {
	installed := tree(t, map[string]string{"core/en/10-invariants.md": "the rules as shipped\n"})
	l := lockOf(t, installed)

	// Upstream revises the file. The lock still describes what was installed, so the
	// installed tree remains unchanged relative to it.
	changes, _ := Diff(installed, l)
	if len(changes) != 0 {
		t.Fatalf("an untouched install must show no local changes regardless of what upstream "+
			"now ships; got %v", changes)
	}
}

func TestMissingLockIsNotAnError(t *testing.T) {
	dir := tree(t, map[string]string{"core/en/a.md": "x\n"})
	l, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l != nil {
		t.Fatal("expected no lock")
	}
	changes, err := Diff(dir, nil)
	if err != nil || changes != nil {
		t.Fatal("a tree installed by an older release has nothing to compare against")
	}
}
