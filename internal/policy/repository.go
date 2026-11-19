package policy

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// repositoryFacts gathers the handful of repository-level facts controls need.
//
// Kept small and named, rather than exposing a general file-system reader to the expression language.
// A pack that could walk arbitrary paths would be code with extra steps, and the closed vocabulary is
// what makes a third-party pack safe to install.
func repositoryFacts(root string) map[string]any {
	facts := map[string]any{
		"tracked_secret_files": trackedSecretFiles(root),
		"client_provider_refs": clientProviderRefs(root),
	}
	return facts
}

// envFilePatterns are the names that carry values rather than names.
//
// `.env.example` is deliberately absent: it exists to be committed, and treating it as a leak would
// mean the honest thing looks like the violation.
var envFilePatterns = []string{".env", ".env.local", ".env.production", ".env.prod", ".env.staging"}

// trackedSecretFiles reports environment files that exist and are not ignored.
//
// It reads .gitignore rather than invoking git, because `check` must work in a directory that is not a
// repository yet — a project running the gate before its first commit is the common case, not an edge
// one.
func trackedSecretFiles(root string) []any {
	ignored := map[string]bool{}
	if raw, err := os.ReadFile(filepath.Join(root, ".gitignore")); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			ignored[strings.TrimPrefix(line, "/")] = true
		}
	}

	var out []any
	for _, name := range envFilePatterns {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			continue
		}
		if ignored[name] || ignored[".env*"] || ignored["*.env"] {
			continue
		}
		out = append(out, name)
	}
	return out
}

// providerMarkers are the ways a browser bundle reaches a model provider.
var providerMarkers = []string{
	"@anthropic-ai/sdk", "from anthropic", "import anthropic",
	"anthropic_api_key", "openai_api_key", "openai/",
}

// clientProviderRefs reports files under the declared client path that reference a provider.
func clientProviderRefs(root string) []any {
	globs := clientGlobs(root)
	var out []any
	for _, g := range globs {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(strings.TrimSuffix(g, "/**"), "/*")))
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch filepath.Ext(p) {
			case ".ts", ".tsx", ".js", ".jsx", ".vue", ".svelte", ".html":
			default:
				return nil
			}
			body, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			lower := strings.ToLower(string(body))
			for _, m := range providerMarkers {
				if strings.Contains(lower, m) {
					rel, _ := filepath.Rel(root, p)
					out = append(out, filepath.ToSlash(rel))
					return nil
				}
			}
			return nil
		})
	}
	return out
}

// clientGlobs reads layout.paths.client with the same small parser used elsewhere.
func clientGlobs(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return nil
	}
	var doc struct {
		Layout struct {
			Paths struct {
				Client []string `yaml:"client"`
			} `yaml:"paths"`
		} `yaml:"layout"`
	}
	if yaml.Unmarshal(raw, &doc) != nil {
		return nil
	}
	return doc.Layout.Paths.Client
}
