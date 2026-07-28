package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tarball(t *testing.T, files map[string]string, mutate func(*tar.Header)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		h := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if mutate != nil {
			mutate(h)
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestVerifyAcceptsAMatchingChecksum(t *testing.T) {
	body := []byte("pack contents")
	if err := Verify(Entry{ID: "x", SHA256: digest(body)}, body); err != nil {
		t.Fatal(err)
	}
}

// The checksum is the whole basis for installing something a stranger published.
func TestVerifyRejectsAReplacedArtifact(t *testing.T) {
	original := []byte("the pack that was reviewed")
	swapped := []byte("something else entirely")

	err := Verify(Entry{ID: "reg.example", SHA256: digest(original)}, swapped)
	if err == nil {
		t.Fatal("a replaced artifact must be rejected")
	}
	if !strings.Contains(err.Error(), "Do not install") {
		t.Errorf("the error should tell the reader what to do, got: %v", err)
	}
}

func TestFetchRefusesAnEntryWithNoChecksum(t *testing.T) {
	if _, err := Fetch(Entry{ID: "x", URL: "https://example.invalid/p.tgz"}, nil, time.Second); err == nil {
		t.Fatal("an entry with no checksum must not be fetched at all")
	}
}

func TestUnpackWritesDataFiles(t *testing.T) {
	data := tarball(t, map[string]string{
		"pack.yaml":                     "schema_version: \"1.0\"\n",
		"controls/agent/something.yaml": "control: {}\n",
		"README.md":                     "# a pack\n",
	}, nil)

	dir := t.TempDir()
	written, err := Unpack(data, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 3 {
		t.Fatalf("wrote %v, want 3 files", written)
	}
	if _, err := os.Stat(filepath.Join(dir, "controls", "agent", "something.yaml")); err != nil {
		t.Error("nested file was not written")
	}
}

// A pack is data. There is no reason for an executable to be inside one, and every reason to
// refuse it — installing a pack must never become a decision about whose code you run.
func TestUnpackRefusesNonDataFiles(t *testing.T) {
	for _, name := range []string{"install.sh", "setup.py", "hook", "bin/agentarch"} {
		data := tarball(t, map[string]string{name: "#!/bin/sh\nid\n"}, nil)
		if _, err := Unpack(data, t.TempDir()); err == nil {
			t.Errorf("archive containing %q must be refused", name)
		}
	}
}

// The classic way to turn an archive into arbitrary write access.
func TestUnpackRefusesPathTraversal(t *testing.T) {
	for _, name := range []string{
		"../escaped.yaml",
		"../../etc/agentarch.yaml",
		"a/../../b.yaml",
	} {
		data := tarball(t, map[string]string{name: "x: 1\n"}, nil)
		dir := t.TempDir()
		if _, err := Unpack(data, dir); err == nil {
			t.Errorf("archive entry %q must be refused", name)
		}
	}
}

func TestUnpackRefusesAbsolutePaths(t *testing.T) {
	data := tarball(t, map[string]string{"/tmp/agentarch-owned.yaml": "x: 1\n"}, nil)
	if _, err := Unpack(data, t.TempDir()); err == nil {
		t.Fatal("an absolute path must be refused")
	}
}

func TestUnpackRefusesSymlinks(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{
		Name: "link.yaml", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777,
	})
	tw.Close()
	gz.Close()

	if _, err := Unpack(buf.Bytes(), t.TempDir()); err == nil {
		t.Fatal("a symlink must be refused")
	}
}

func TestIndexProblemsRequireVerifiability(t *testing.T) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	idx := &Index{Entries: []Entry{
		{ID: "a.no-checksum", URL: "https://x", Owner: "o", Licence: "CC-BY-4.0", Trust: "community"},
		{ID: "b.no-owner", URL: "https://x", SHA256: "aa", Licence: "CC-BY-4.0", Trust: "community"},
		{ID: "c.stale", URL: "https://x", SHA256: "aa", Owner: "o", Licence: "CC-BY-4.0",
			Trust: "community", ReviewedAt: "2024-01-01"},
	}}
	problems := idx.Problems(now)
	if len(problems) < 3 {
		t.Fatalf("expected at least 3 problems, got %v", problems)
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"no checksum", "no owner", "more than a year"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems should mention %q, got:\n%s", want, joined)
		}
	}
}

// The shipped index must itself be clean, or it is teaching adopters that the fields are
// decorative.
func TestShippedIndexIsClean(t *testing.T) {
	d, _ := os.Getwd()
	for range 5 {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			break
		}
		d = filepath.Dir(d)
	}
	idx, err := LoadIndex(filepath.Join(d, "registry", "index.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if p := idx.Problems(time.Now().UTC()); len(p) > 0 {
		t.Fatalf("the shipped registry has problems: %v", p)
	}
}
