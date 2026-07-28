// Package lockfile records what was installed, so a local edit can be told apart from an
// upstream change.
//
// Comparing an installed tree against the payload a newer binary carries cannot distinguish the
// two: every file the standard changed upstream looks exactly like a file the project edited.
// The lock records the hash of each file *as installed*, which is the only reference that
// answers the question being asked.
package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Name is the file written into agentarch/std/.
const Name = "LOCK.json"

// Lock is the record written at install time.
type Lock struct {
	Version   string            `json:"content_version"`
	CreatedAt string            `json:"installed_at"`
	Files     map[string]string `json:"files"`
}

// Hash reduces a file's contents to a comparable value.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Build walks an installed tree and records every file except the lock itself.
func Build(stdDir, version, installedAt string) (*Lock, error) {
	l := &Lock{Version: version, CreatedAt: installedAt, Files: map[string]string{}}

	err := filepath.WalkDir(stdDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(stdDir, p)
		if rerr != nil {
			return rerr
		}
		if rel == Name {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		l.Files[filepath.ToSlash(rel)] = Hash(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Write saves the lock into the installed tree.
func Write(stdDir string, l *Lock) error {
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	header := []byte("")
	_ = header
	return os.WriteFile(filepath.Join(stdDir, Name), append(b, '\n'), 0o644)
}

// Read loads the lock. A missing lock is not an error: a tree installed by an older release
// simply has nothing to compare against.
func Read(stdDir string) (*Lock, error) {
	b, err := os.ReadFile(filepath.Join(stdDir, Name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var l Lock
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// Change is one difference between the lock and what is on disk.
type Change struct {
	Path string
	Kind string // "edited", "added", "deleted"
}

// Diff compares an installed tree against its lock.
//
// Added files are reported separately from edits because they behave differently on upgrade: an
// edit to a vendored file is lost, while a file the project added under std/ survives and then
// silently shadows nothing — which is its own kind of confusing.
func Diff(stdDir string, l *Lock) ([]Change, error) {
	if l == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []Change

	err := filepath.WalkDir(stdDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(stdDir, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if rel == Name {
			return nil
		}
		seen[rel] = true

		want, known := l.Files[rel]
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		switch {
		case !known:
			out = append(out, Change{rel, "added"})
		case Hash(b) != want:
			out = append(out, Change{rel, "edited"})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for rel := range l.Files {
		if !seen[rel] {
			out = append(out, Change{rel, "deleted"})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// BuildFromFS records an embedded payload without touching disk, for tests.
func BuildFromFS(fsys fs.FS, root, version, installedAt string) (*Lock, error) {
	l := &Lock{Version: version, CreatedAt: installedAt, Files: map[string]string{}}
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := fs.ReadFile(fsys, p)
		if rerr != nil {
			return rerr
		}
		l.Files[strings.TrimPrefix(p, root+"/")] = Hash(b)
		return nil
	})
	return l, err
}
