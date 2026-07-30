// Layer dependency checking: AA-DEP-019.
//
// This is the executable half of control.ai.api.core_transport_separated, and the one lint here that
// reads the adopting project rather than the standard's own content. It is deliberately modest about
// what it can prove.
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout is the declared shape of a project, from agentarch.yaml.
type Layout struct {
	Edge   []string
	Core   []string
	Client []string
}

// forbiddenInCore is what an agent core must not reach for.
//
// Two kinds of entry, and the distinction matters. The path prefixes are derived from the declared
// edge globs at call time. These are the package names that indicate a transport regardless of where
// it was imported from — a core that imports a web framework directly has the same problem as one
// that imports the route module, because either way it can only run inside a server.
var forbiddenPackages = []string{
	// Python
	"fastapi", "starlette", "flask", "django", "aiohttp.web", "tornado.web", "sanic", "quart",
	// JS/TS
	"express", "next/server", "@nestjs/common", "fastify", "koa", "hono",
	// Go
	"net/http", "github.com/gin-gonic/gin", "github.com/labstack/echo", "github.com/gofiber/fiber",
	// Java / .NET
	"javax.servlet", "jakarta.servlet", "org.springframework.web", "Microsoft.AspNetCore",
}

// LintLayers reports imports that cross a declared layer boundary the wrong way.
//
// What it catches: the mistake people actually make — reaching into the transport from the agent to
// read a header, or importing the web framework so a decorator can be used somewhere convenient.
//
// What it does not catch, said plainly rather than implied: dynamic imports, imports assembled from
// strings, dependency injection that hands the core a request object at runtime, and any language
// whose import syntax is not a line starting with import/from/require/using. A check that claimed to
// prove the absence of a dependency would be worse than one that admits its reach, because the first
// invites you to stop reading the code.
func LintLayers(root string, layout Layout) []Finding {
	if len(layout.Core) == 0 || len(layout.Edge) == 0 {
		// Nothing declared, nothing to check. The control reports the missing declaration itself;
		// inventing a default here would check a layout the project never agreed to.
		return nil
	}

	edgeModules := modulePrefixes(layout.Edge)
	var out []Finding

	for _, file := range filesMatching(root, layout.Core) {
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, file)
		for i, line := range strings.Split(string(body), "\n") {
			stripped := strings.TrimSpace(line)
			if !isImport(stripped) {
				continue
			}
			if what := offendingImport(stripped, edgeModules); what != "" {
				out = append(out, Finding{
					ID:      "AA-DEP-019",
					File:    filepath.ToSlash(rel),
					Pointer: fmt.Sprintf("line %d", i+1),
					Message: "the agent core imports " + what +
						", which belongs to the transport layer",
					Fix: "Move what the handler needed into a value the core defines, and let the " +
						"transport construct it. A core that depends on the transport cannot be run " +
						"from a test, a queue worker or a CLI — see control.ai.api.core_transport_separated.",
				})
			}
		}
	}

	// The client is checked for the opposite thing: not a layer violation but a credential. A browser
	// bundle that talks to a provider directly has no input check, no output check, no tool
	// authorisation, no budget and no audit trail.
	for _, file := range filesMatching(root, layout.Client) {
		body, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, file)
		for i, line := range strings.Split(string(body), "\n") {
			lower := strings.ToLower(line)
			for _, marker := range []string{
				"@anthropic-ai/sdk", "from anthropic", "import anthropic",
				"openai", "anthropic_api_key", "openai_api_key",
			} {
				if strings.Contains(lower, marker) {
					out = append(out, Finding{
						ID:      "AA-DEP-019",
						File:    filepath.ToSlash(rel),
						Pointer: fmt.Sprintf("line %d", i+1),
						Message: "a web client references a model provider (" + marker + ")",
						Fix: "Route it through your own endpoint: the client sends a question and " +
							"receives an answer. If a credential reached a bundle, rotate it — it " +
							"has been served to every visitor.",
					})
					break
				}
			}
		}
	}

	return out
}

func isImport(s string) bool {
	for _, p := range []string{"import ", "from ", "require(", "using ", "const ", "let ", "var "} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// offendingImport returns what the line reaches for, or empty.
func offendingImport(line string, edgeModules []string) string {
	for _, m := range edgeModules {
		// Both spellings: `from app.api.deps import x` and `from ..api import deps`.
		if strings.Contains(line, m+".") || strings.Contains(line, m+" ") ||
			strings.Contains(line, "/"+m+"/") || strings.HasSuffix(line, m) {
			return m
		}
	}
	// A relative import that climbs out of the core and into a sibling named like the edge.
	for _, m := range edgeModules {
		if strings.Contains(line, ".."+m) || strings.Contains(line, "../"+m) {
			return m
		}
	}
	for _, p := range forbiddenPackages {
		if strings.Contains(line, p) {
			return p
		}
	}
	return ""
}

// modulePrefixes turns declared globs into importable module names: app/api/** -> app.api and api.
func modulePrefixes(globs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range globs {
		clean := strings.TrimSuffix(strings.TrimSuffix(g, "/**"), "/*")
		clean = strings.Trim(clean, "/")
		if clean == "" {
			continue
		}
		dotted := strings.ReplaceAll(clean, "/", ".")
		leaf := clean
		if i := strings.LastIndex(clean, "/"); i >= 0 {
			leaf = clean[i+1:]
		}
		for _, candidate := range []string{dotted, leaf} {
			if candidate != "" && !seen[candidate] {
				seen[candidate] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

// filesMatching walks the globs and returns source files under them.
func filesMatching(root string, globs []string) []string {
	var out []string
	for _, g := range globs {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(strings.TrimSuffix(g, "/**"), "/*")))
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch filepath.Ext(p) {
			case ".py", ".ts", ".tsx", ".js", ".jsx", ".go", ".java", ".cs", ".rb", ".kt":
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}
