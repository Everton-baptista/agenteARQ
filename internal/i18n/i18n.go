// Package i18n keeps translations honest about which source they were made from.
//
// A stale translation is worse than a missing one. A missing translation sends the reader to
// the English source; a stale one answers their question with authority, using a rule that has
// since changed. Every translated file therefore records the SHA-256 of the source it was made
// from, and drift is a finding rather than something a reader has to notice.
package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceLang is normative. Translations are derived from it and never the other way round —
// with two editable sources there is no answer to which one was right.
const SourceLang = "en"

// FrontMatter is the header every translated file carries.
type FrontMatter struct {
	Lang         string   `yaml:"lang"`
	Source       string   `yaml:"source"`
	SourceSHA256 string   `yaml:"source_sha256"`
	TranslatedAt string   `yaml:"translated_at"`
	Translators  []string `yaml:"translators"`
}

// Status is the state of one translated file.
type Status struct {
	Path       string
	Lang       string
	SourcePath string
	Stale      bool
	Missing    bool // the file declares a source that does not exist
	NoHeader   bool
	Want       string // digest of the source as it stands now
	Got        string // digest the translation was made from
}

// Digest is how a source file is reduced to a comparable value.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var fmDelim = "---"

// ParseFrontMatter splits a YAML front-matter header from the body.
func ParseFrontMatter(content string) (FrontMatter, string, bool) {
	if !strings.HasPrefix(content, fmDelim) {
		return FrontMatter{}, content, false
	}
	rest := content[len(fmDelim):]
	rest = strings.TrimPrefix(rest, "\n")
	end := strings.Index(rest, "\n"+fmDelim)
	if end < 0 {
		return FrontMatter{}, content, false
	}
	head := rest[:end]
	// Trim every blank line between the header and the first heading, so callers get the
	// document as if the bookkeeping were not there.
	body := strings.TrimLeft(rest[end+len(fmDelim)+1:], "\n")

	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(head), &fm); err != nil {
		return FrontMatter{}, content, false
	}
	return fm, body, true
}

// Check walks every non-source language under a content tree and reports drift.
//
// Directories checked are content/core/<lang> and content/standards/<lang>; references are
// deliberately not checked, because they are pointers to external material rather than
// normative text.
func Check(fsys fs.FS) ([]Status, error) {
	var out []Status

	for _, area := range []string{"core", "standards"} {
		langs, err := fs.ReadDir(fsys, area)
		if err != nil {
			continue // an area with no translations at all is fine
		}
		for _, l := range langs {
			if !l.IsDir() || l.Name() == SourceLang {
				continue
			}
			dir := path.Join(area, l.Name())
			entries, err := fs.ReadDir(fsys, dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				p := path.Join(dir, e.Name())
				raw, err := fs.ReadFile(fsys, p)
				if err != nil {
					return nil, err
				}
				st := Status{Path: p, Lang: l.Name()}

				fm, _, ok := ParseFrontMatter(string(raw))
				if !ok || fm.SourceSHA256 == "" {
					st.NoHeader = true
					out = append(out, st)
					continue
				}

				// The declared source is relative to the repository, so strip the
				// content/ prefix the payload does not carry.
				src := strings.TrimPrefix(fm.Source, "content/")
				st.SourcePath = src
				st.Got = fm.SourceSHA256

				srcRaw, err := fs.ReadFile(fsys, src)
				if err != nil {
					st.Missing = true
					out = append(out, st)
					continue
				}
				st.Want = Digest(srcRaw)
				if st.Want != st.Got {
					st.Stale = true
				}
				out = append(out, st)
			}
		}
	}
	return out, nil
}

// Problems filters to the entries a reader would be misled by.
func Problems(all []Status) []Status {
	var out []Status
	for _, s := range all {
		if s.Stale || s.Missing || s.NoHeader {
			out = append(out, s)
		}
	}
	return out
}

func (s Status) String() string {
	switch {
	case s.NoHeader:
		return fmt.Sprintf("AA-I18N-016  %s\n    no translation header — cannot tell which source this was made from\n"+
			"    fix: add lang, source and source_sha256 front matter", s.Path)
	case s.Missing:
		return fmt.Sprintf("AA-I18N-016  %s\n    declares source %q, which does not exist\n"+
			"    fix: point at the current source file, or remove the translation", s.Path, s.SourcePath)
	default:
		return fmt.Sprintf("AA-I18N-016  %s\n    source has changed since translation: made from %s…, source is now %s…\n"+
			"    fix: retranslate and update source_sha256. A stale translation answers the\n"+
			"    reader's question with authority using a rule that has since changed.",
			s.Path, s.Got[:12], s.Want[:12])
	}
}
