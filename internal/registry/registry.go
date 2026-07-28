// Package registry installs community packs, adapters and translations.
//
// This is the only part of agentarch that fetches anything, and it is opt-in for that reason.
// Two rules hold regardless of who published an entry: the checksum is verified before anything
// is written to disk, and nothing from a pack is ever executed. The second is what the
// expression language exists to guarantee — a pack is data, so installing one from a stranger
// is a decision about which rules you accept, not about whose code you run.
package registry

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Entry is one listed artifact.
type Entry struct {
	ID              string   `yaml:"id"`
	Kind            string   `yaml:"kind"` // pack | adapter | translation
	Title           string   `yaml:"title"`
	Version         string   `yaml:"version"`
	URL             string   `yaml:"url"`
	SHA256          string   `yaml:"sha256"`
	Signature       string   `yaml:"signature"`
	Licence         string   `yaml:"licence"`
	Owner           string   `yaml:"owner"`
	Trust           string   `yaml:"trust"`
	ReviewedAt      string   `yaml:"reviewed_at"`
	SpecVersion     string   `yaml:"spec_version"`
	AuthorityStatus string   `yaml:"authority_status"`
	Jurisdiction    []string `yaml:"jurisdiction"`
}

// Index is the registry document.
type Index struct {
	SchemaVersion string            `yaml:"schema_version"`
	TrustLevels   map[string]string `yaml:"trust_levels"`
	Entries       []Entry           `yaml:"entries"`
}

// LoadIndex reads a registry index from disk.
func LoadIndex(path string) (*Index, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := yaml.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &idx, nil
}

// Find looks an entry up by id.
func (i *Index) Find(id string) (Entry, bool) {
	for _, e := range i.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Problems reports entries that cannot be trusted as listed. A registry listing things nobody
// can verify teaches people that the checksums are decorative.
func (i *Index) Problems(now time.Time) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range i.Entries {
		switch {
		case e.ID == "":
			out = append(out, "an entry has no id")
		case seen[e.ID]:
			out = append(out, e.ID+": listed twice")
		case e.SHA256 == "":
			out = append(out, e.ID+": no checksum, so nothing about it can be verified")
		case e.URL == "":
			out = append(out, e.ID+": no url")
		case e.Owner == "":
			out = append(out, e.ID+": no owner")
		case e.Licence == "":
			out = append(out, e.ID+": no licence")
		case e.Trust == "":
			out = append(out, e.ID+": no trust level; say plainly what is known about it")
		}
		seen[e.ID] = true

		if e.ReviewedAt != "" {
			if d, err := time.Parse("2006-01-02", e.ReviewedAt); err == nil &&
				now.Sub(d) > 365*24*time.Hour {
				out = append(out, e.ID+": last reviewed more than a year ago")
			}
		}
	}
	return out
}

// MaxArtifactBytes bounds a download. An entry that streams forever must not exhaust the machine.
const MaxArtifactBytes = 32 << 20

// Fetch downloads an entry and verifies its checksum.
//
// Verification happens over the bytes in memory, before anything reaches the filesystem. A
// checksum checked after unpacking is a checksum checked after the damage.
func Fetch(e Entry, client *http.Client, timeout time.Duration) ([]byte, error) {
	if e.SHA256 == "" {
		return nil, fmt.Errorf("%s has no checksum; refusing to fetch", e.ID)
	}
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Get(e.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", e.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxArtifactBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", e.ID, MaxArtifactBytes)
	}
	return body, Verify(e, body)
}

// Verify checks a downloaded artifact against the checksum the index declares.
func Verify(e Entry, body []byte) error {
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	want := strings.TrimPrefix(e.SHA256, "sha256-")
	if got != want {
		return fmt.Errorf(
			"checksum mismatch for %s: index says %s…, artifact hashes to %s…\n"+
				"Do not install this. Either the registry entry is wrong or the artifact was replaced.",
			e.ID, short(want), short(got))
	}
	return nil
}

// Unpack extracts a verified tarball into dest.
//
// Only .yaml and .md regular files are written. An archive is untrusted input, and the classic
// way to turn one into arbitrary write access is a path that escapes the destination — so
// absolute paths, traversal, symlinks and device nodes are all refused. A pack is data; there is
// no reason for a script or a binary to be inside one, and every reason to refuse it.
func Unpack(data []byte, dest string) ([]string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var written []string
	cleanDest := filepath.Clean(dest)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return written, err
		}

		clean := filepath.Clean(h.Name)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return written, fmt.Errorf("archive contains an escaping path: %q", h.Name)
		}
		target := filepath.Join(cleanDest, clean)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return written, fmt.Errorf("archive entry %q would write outside the destination", h.Name)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return written, err
			}
		case tar.TypeReg:
			if !strings.HasSuffix(target, ".yaml") && !strings.HasSuffix(target, ".md") {
				return written, fmt.Errorf(
					"archive contains %q; a pack may only contain .yaml and .md files", h.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return written, err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return written, err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 8<<20)); err != nil {
				f.Close()
				return written, err
			}
			f.Close()
			written = append(written, clean)
		default:
			return written, fmt.Errorf(
				"archive entry %q is not a regular file or directory", h.Name)
		}
	}
	return written, nil
}

func short(s string) string {
	s = strings.TrimPrefix(s, "sha256-")
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
