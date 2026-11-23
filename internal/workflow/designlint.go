package workflow

// design.tokens_only, checked on the stylesheets themselves.
//
// A recorded design direction only holds if the interface uses it. The first pilot product
// passed every check while its stylesheet set a field background to a literal #ffffff that no
// token named — found by the reviewer, not by a check. Colours are the property that drifts
// first, so this reads every stylesheet of a product that has design tokens and reports each
// colour written outside a custom-property definition. It is advisory, like every new check, and
// it reads files only.

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DesignAdvicePrefix starts every design-lint advice line.
const DesignAdvicePrefix = "Design (advisory): "

const designLintMaxFindings = 5

var (
	cssComment     = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssDeclaration = regexp.MustCompile(`([-A-Za-z]+)\s*:\s*([^;{}]+)`)
	cssColour      = regexp.MustCompile(`#[0-9A-Fa-f]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb|color)\(`)
)

// tokenColourAdvice lists literal colours used outside token definitions in the product's CSS.
func (s *Service) tokenColourAdvice() []string {
	if !s.exists(DesignTokensPath) {
		return nil
	}
	var findings []string
	total := 0
	_ = filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(s.Root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != s.Root && (riskSkipDirs[d.Name()] || rel == "agentarch/std") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".css") || strings.HasSuffix(d.Name(), ".min.css") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > riskMaxBytes {
			return nil
		}
		// Blank out comments but keep their newlines, so line numbers stay true.
		text := cssComment.ReplaceAllStringFunc(string(b), func(c string) string {
			return strings.Repeat("\n", strings.Count(c, "\n"))
		})
		for i, line := range strings.Split(text, "\n") {
			for _, m := range cssDeclaration.FindAllStringSubmatch(line, -1) {
				if strings.HasPrefix(m[1], "--") {
					continue // a token definition is where a literal belongs
				}
				if c := cssColour.FindString(m[2]); c != "" {
					total++
					if len(findings) < designLintMaxFindings {
						findings = append(findings, fmt.Sprintf("%s%s:%d sets %s to the literal colour %s; use a token variable so the interface keeps the recorded direction.", DesignAdvicePrefix, rel, i+1, m[1], strings.TrimSuffix(c, "(")))
					}
				}
			}
		}
		return nil
	})
	if total > len(findings) {
		findings = append(findings, fmt.Sprintf("%s%d more literal colours in stylesheets.", DesignAdvicePrefix, total-len(findings)))
	}
	return findings
}
