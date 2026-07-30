// OpenAPI generation from the manifest's interface block.
//
// The same relationship .mcp.json has with the MCP allowlist: the reviewed document is the source and
// the machine file is the derivative. Two hand-maintained descriptions of one interface diverge, and
// the consumer reads whichever is wrong.
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// InterfaceOf is the subset of a manifest this reads.
type InterfaceOf struct {
	AgentID   string
	Purpose   string
	Transport string
	BasePath  string
	Auth      string
	Routes    []Route
}

type Route struct {
	Path         string
	Method       string
	Summary      string
	AuthRequired bool
	Idempotent   bool
}

// securitySchemes maps the manifest's vocabulary onto OpenAPI's.
//
// `none` is present and produces no scheme, because anonymous access is a decision worth being able
// to declare rather than something you get by omission.
var securitySchemes = map[string]map[string]any{
	"bearer_jwt":     {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
	"api_key":        {"type": "apiKey", "in": "header", "name": "X-API-Key"},
	"oauth2":         {"type": "oauth2", "flows": map[string]any{}},
	"session_cookie": {"type": "apiKey", "in": "cookie", "name": "session"},
	"mtls":           {"type": "mutualTLS"},
	"iam":            {"type": "http", "scheme": "bearer"},
}

// BuildOpenAPI renders the document and its source digest.
//
// The digest covers the interface block, not the rendered file, so reformatting the JSON does not read
// as a change to the contract while an added route does.
func BuildOpenAPI(iface InterfaceOf, version string) ([]byte, string, error) {
	if iface.Transport != "http" {
		return nil, "", fmt.Errorf(
			"interface.transport is %q, so there is no HTTP contract to generate.\n"+
				"A cli, queue or library agent has no endpoints; remove layout.paths.contract or "+
				"declare transport: http", iface.Transport)
	}
	if len(iface.Routes) == 0 {
		return nil, "", fmt.Errorf(
			"interface.routes is empty, so the generated contract would describe nothing.\n" +
				"Declare the endpoints in the manifest — that is the reviewed source, and this file " +
				"is derived from it")
	}

	sum := sha256.Sum256([]byte(digestSource(iface)))
	digest := hex.EncodeToString(sum[:])

	paths := map[string]any{}
	// Sorted, so the same manifest produces the same bytes. Go map ordering is deliberately random,
	// and a file that differs between two runs makes `sync --check` report drift that never happened.
	routes := append([]Route(nil), iface.Routes...)
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})

	for _, r := range routes {
		full := strings.TrimSuffix(iface.BasePath, "/") + r.Path
		item, _ := paths[full].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[full] = item
		}

		op := map[string]any{
			"operationId": operationID(r),
			"summary":     r.Summary,
			"responses": map[string]any{
				"200": map[string]any{"description": "the request was handled"},
			},
		}
		if r.AuthRequired {
			// Named at the operation rather than only at the document, so a reader can see per
			// endpoint whether it is open. A global `security` block plus one exception is how an
			// endpoint ends up unauthenticated without anyone noticing.
			op["security"] = []map[string][]string{{schemeName(iface.Auth): {}}}
			resp, _ := op["responses"].(map[string]any)
			resp["401"] = map[string]any{"description": "no or invalid credential"}
			resp["429"] = map[string]any{"description": "the caller's budget is exhausted"}
		} else {
			op["security"] = []map[string][]string{}
		}
		if !r.Idempotent {
			// Recorded because a caller that retries a non-idempotent call performs it twice, and for
			// an irreversible action that is the whole failure.
			op["x-agentarch-idempotent"] = false
		}
		item[strings.ToLower(r.Method)] = op
	}

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       iface.AgentID,
			"version":     version,
			"description": iface.Purpose,
			// A generated file that does not say so gets hand-edited. This one names the source and
			// the command that rebuilds it.
			"x-agentarch-generated":     "DO NOT EDIT — generated from agent.yaml interface by `agentarch sync`",
			"x-agentarch-source-sha256": digest,
		},
		"paths": paths,
	}
	if scheme, ok := securitySchemes[iface.Auth]; ok {
		doc["components"] = map[string]any{
			"securitySchemes": map[string]any{schemeName(iface.Auth): scheme},
		}
	}

	// Indented and newline-terminated: this file is committed and reviewed in diffs, and a
	// single-line JSON document makes a one-route change look like a rewrite.
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return append(out, '\n'), digest, nil
}

// digestSource is the canonical form the digest is taken over.
//
// Only the fields that change the contract. Adding the summary would mean a wording fix reads as an
// interface change; leaving out the method would mean adding a DELETE to an existing path does not.
func digestSource(iface InterfaceOf) string {
	parts := []string{iface.Transport, iface.BasePath, iface.Auth}
	var routes []string
	for _, r := range iface.Routes {
		routes = append(routes, fmt.Sprintf("%s %s auth=%t idem=%t",
			strings.ToUpper(r.Method), r.Path, r.AuthRequired, r.Idempotent))
	}
	sort.Strings(routes)
	return strings.Join(append(parts, routes...), "\n")
}

// SourceDigestOf reads the digest recorded in a generated contract. Empty when absent, which is how a
// hand-written file is told apart from a stale generated one.
func SourceDigestOf(content []byte) string {
	var doc struct {
		Info struct {
			Digest string `json:"x-agentarch-source-sha256"`
		} `json:"info"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return ""
	}
	return doc.Info.Digest
}

func operationID(r Route) string {
	clean := strings.NewReplacer("/", "_", "{", "", "}", "", "-", "_").Replace(strings.Trim(r.Path, "/"))
	return strings.ToLower(r.Method) + "_" + clean
}

func schemeName(auth string) string {
	if auth == "" {
		return "none"
	}
	return auth
}
