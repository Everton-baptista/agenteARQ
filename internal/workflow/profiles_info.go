package workflow

import (
	"fmt"
	"strings"
)

// ProfileInfo says what an engineering profile is for and what it checks, so a person can
// compare profiles instead of being handed one.
type ProfileInfo struct {
	ID     string `json:"id"`
	Fits   string `json:"fits"`
	Checks string `json:"checks"`
}

var profileInfo = []ProfileInfo{
	{"typescript-web", "a TypeScript web app with npm scripts", "lint, typecheck, unit and e2e tests, build, accessibility, security"},
	{"static-site", "HTML, CSS and JavaScript served as files, no build step", "HTML validity, JavaScript lint, broken local links"},
	{"python-api", "a Python service or library", "ruff, mypy, pytest, pip check, bandit"},
	{"go-service", "a Go module", "gofmt, go vet, build, tests with the race detector"},
	{"docs", "documentation, standards or diagrams as text", "Markdown structure, broken local links and images"},
	{"universal", "any other stack (mobile, Java, .NET, Rust, data pipelines…): you name the commands", "only the checks you declare in product.yaml"},
}

// ProfileOptions lists the profiles this build ships, in a stable order, with their purpose.
// Every one of them also runs the same security floor.
func ProfileOptions() []ProfileInfo {
	var out []ProfileInfo
	for _, p := range profileInfo {
		if _, ok := profiles[p.ID]; ok {
			out = append(out, p)
		}
	}
	return out
}

// ProfileChoiceText is the comparison shown whenever a profile has to be chosen.
func ProfileChoiceText() string {
	var b strings.Builder
	for _, p := range ProfileOptions() {
		fmt.Fprintf(&b, "  %-15s %s\n      checks: %s\n", p.ID, p.Fits, p.Checks)
	}
	b.WriteString("  Every profile also runs the security floor: secrets and known-vulnerable dependencies.\n")
	return b.String()
}
