package workflow

// Importing a specification written with another spec-driven tool.
//
// AgenteARQ is meant to sit under whatever framework a team already writes specs with — Spec
// Kit, OpenSpec, Kiro — and add what those keep inside the model: gates, isolated evidence and
// independent review. So their requirements are read here and numbered as acceptance criteria,
// each citing the file and line it came from, rather than retyped by an assistant that might
// paraphrase them.
//
// The import is deterministic and conservative. It reads only; it takes the requirement and
// scenario statements those formats mark explicitly (SHALL/MUST requirements, WHEN/THEN and
// Given/When/Then scenarios); it never overwrites a specification someone has filled in, writing
// spec.imported.md beside it instead for the owner to merge.

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ImportedSpecPath is written when spec.md already has content of its own.
const ImportedSpecPath = "agentarch/project/spec.imported.md"

// SpecSource is one detected format and the files it contributes.
type SpecSource struct {
	Format string   `json:"format"`
	Files  []string `json:"files"`
}

// ImportResult says what was found and where it was written.
type ImportResult struct {
	Sources  []SpecSource `json:"sources"`
	Criteria int          `json:"criteria"`
	Written  string       `json:"written"`
}

var specFormats = []struct {
	name  string
	globs []string
}{
	{"kiro", []string{".kiro/specs/*/requirements.md"}},
	{"spec-kit", []string{"specs/*/spec.md"}},
	{"openspec", []string{"openspec/specs/*/spec.md", "openspec/specs/*/*/spec.md", "openspec/changes/*/specs/*/spec.md"}},
}

var (
	requirementLine = regexp.MustCompile(`\b(SHALL|MUST)\b`)
	scenarioLine    = regexp.MustCompile(`(?i)^\W*(\*\*)?(given|when|then)\b.*\b(then)\b`)
	listMarker      = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+`)
	emphasis        = regexp.MustCompile(`\*\*([^*]+)\*\*`)
)

// DetectSpecSources finds specification files of the known formats. from narrows it to one
// format; "" or "auto" looks for all of them.
func DetectSpecSources(root, from string) ([]SpecSource, error) {
	var out []SpecSource
	known := false
	for _, f := range specFormats {
		if from != "" && from != "auto" && from != f.name {
			continue
		}
		known = true
		seen := map[string]bool{}
		var files []string
		for _, g := range f.globs {
			matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(g)))
			if err != nil {
				return nil, err
			}
			for _, m := range matches {
				if !insideRoot(root, m) {
					continue // a symlinked spec folder must not pull in files from elsewhere
				}
				rel, _ := filepath.Rel(root, m)
				rel = filepath.ToSlash(rel)
				if !seen[rel] {
					seen[rel] = true
					files = append(files, rel)
				}
			}
		}
		sort.Strings(files)
		if len(files) > 0 {
			out = append(out, SpecSource{Format: f.name, Files: files})
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown spec format %q (choose kiro, spec-kit or openspec)", from)
	}
	return out, nil
}

type importedCriterion struct {
	text, source string
}

// extractCriteria reads the requirement and scenario statements of one file.
func extractCriteria(root, rel string) ([]importedCriterion, error) {
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []importedCriterion
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	line := 0
	inFence := false
	for sc.Scan() {
		line++
		raw := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			inFence = !inFence
			continue
		}
		if inFence || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			continue
		}
		text := strings.TrimSpace(listMarker.ReplaceAllString(raw, ""))
		text = emphasis.ReplaceAllString(text, "$1")
		if text == "" || len(text) > 600 {
			continue
		}
		if requirementLine.MatchString(text) || scenarioLine.MatchString(text) {
			out = append(out, importedCriterion{text: text, source: fmt.Sprintf("%s:%d", rel, line)})
		}
	}
	return out, sc.Err()
}

// ImportSpec writes the detected requirements as numbered acceptance criteria.
func (s *Service) ImportSpec(from string) (ImportResult, error) {
	var r ImportResult
	unlock, err := s.lock()
	if err != nil {
		return r, err
	}
	defer unlock()
	sources, err := DetectSpecSources(s.Root, from)
	if err != nil {
		return r, err
	}
	if len(sources) == 0 {
		return r, errors.New("no Spec Kit (specs/*/spec.md), OpenSpec (openspec/specs/**/spec.md) or Kiro (.kiro/specs/*/requirements.md) specification found")
	}
	r.Sources = sources
	var criteria []importedCriterion
	for _, src := range sources {
		for _, rel := range src.Files {
			c, err := extractCriteria(s.Root, rel)
			if err != nil {
				return r, err
			}
			criteria = append(criteria, c...)
		}
	}
	if len(criteria) == 0 {
		return r, errors.New("the specification files contain no SHALL/MUST requirement or WHEN/THEN scenario to import")
	}
	var b strings.Builder
	b.WriteString("# Specification\n\nImported by `agentarch workflow import-spec` from ")
	var names []string
	for _, src := range sources {
		names = append(names, src.Format)
	}
	b.WriteString(strings.Join(names, ", "))
	b.WriteString(". Each criterion cites the line it came from; review them, merge duplicates and make each one observable from outside the code.\n\n")
	for i, c := range criteria {
		fmt.Fprintf(&b, "- AC-%d: %s (from %s)\n", i+1, c.text, c.source)
	}
	r.Criteria = len(criteria)

	target := documents[1]
	path, err := safePath(s.Root, target)
	if err != nil {
		return r, err
	}
	if existing, err := os.ReadFile(path); err == nil && !isUntouchedSpec(string(existing)) {
		target = ImportedSpecPath
		if path, err = safePath(s.Root, target); err != nil {
			return r, err
		}
		if _, err := os.Lstat(path); err == nil {
			return r, fmt.Errorf("%s already exists; merge it into spec.md or delete it, then import again", ImportedSpecPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return r, err
	}
	if err := atomicWrite(path, []byte(b.String()), fs.FileMode(0o644)); err != nil {
		return r, err
	}
	r.Written = target
	return r, nil
}

// isUntouchedSpec reports whether spec.md is still exactly a template, the only case the import
// may replace it; anything the owner or an assistant has written stays where it is.
func isUntouchedSpec(spec string) bool {
	norm := strings.TrimSpace(spec)
	return norm == "" || norm == strings.TrimSpace(SpecTemplate) || norm == strings.TrimSpace(SpecTemplatePT)
}

func insideRoot(root, path string) bool {
	r, err1 := filepath.EvalSymlinks(root)
	p, err2 := filepath.EvalSymlinks(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
