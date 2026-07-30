package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Everton-baptista/agenteARQ/internal/render"
)

// syncOpenAPI derives contracts/openapi.json from the manifests' interface blocks.
//
// One file for the project rather than one per agent, because the contract describes what a caller can
// reach — and a caller reaching two agents through one base path is reading one interface, whatever the
// repository's internal division. An agent with `transport: library` contributes nothing: it is reached
// by a declared handoff, never from outside, and inventing endpoints for it would describe a surface
// that does not exist.
func syncOpenAPI(root string, check bool) (int, int) {
	dst, err := contractPath(root)
	if err != nil || dst == "" {
		// No layout.paths.contract, so the project has not asked for a generated contract. The
		// control reports the missing declaration; generating one anyway would put a file in
		// somebody's repository because we guessed a path.
		return 0, exitOK
	}

	ifaces, err := interfacesOf(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}
	if len(ifaces) == 0 {
		return 0, exitOK
	}

	// Merged in agent-id order so the same project produces the same bytes.
	merged := ifaces[0]
	for _, next := range ifaces[1:] {
		merged.Routes = append(merged.Routes, next.Routes...)
	}
	if len(ifaces) > 1 {
		merged.AgentID = filepath.Base(root)
		merged.Purpose = fmt.Sprintf("%d agents reachable at %s", len(ifaces), merged.BasePath)
	}

	want, _, err := render.BuildOpenAPI(merged, "1.0.0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}

	full := filepath.Join(root, filepath.FromSlash(dst))
	existing, _ := os.ReadFile(full)
	if string(existing) == string(want) {
		return 0, exitOK
	}
	if check {
		reason := "content differs from the interface block in agent.yaml"
		if len(existing) == 0 {
			reason = "file is missing"
		}
		fmt.Fprintf(os.Stderr, "drift  %s\n    %s\n", dst, reason)
		return 1, exitOK
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}
	if err := os.WriteFile(full, want, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "sync:", err)
		return 0, exitUsage
	}
	fmt.Printf("wrote %s (%d route(s) from %d manifest(s))\n", dst, len(merged.Routes), len(ifaces))
	return 0, exitOK
}

// contractPath reads layout.paths.contract.
func contractPath(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if err != nil {
		return "", err
	}
	var doc struct {
		Layout struct {
			Paths struct {
				Contract string `yaml:"contract"`
			} `yaml:"paths"`
		} `yaml:"layout"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	return doc.Layout.Paths.Contract, nil
}

// interfacesOf reads every manifest that declares an HTTP interface, in agent-id order.
func interfacesOf(root string) ([]render.InterfaceOf, error) {
	base := filepath.Join(root, "agentarch", "project", "agents")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, nil
	}

	var out []render.InterfaceOf
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(base, e.Name(), "agent.yaml"))
		if err != nil {
			continue
		}
		var doc struct {
			Agent struct {
				ID        string `yaml:"id"`
				Purpose   string `yaml:"purpose"`
				Interface struct {
					Transport string `yaml:"transport"`
					BasePath  string `yaml:"base_path"`
					Caller    struct {
						IdentifiedBy string `yaml:"identified_by"`
					} `yaml:"caller"`
					Routes []struct {
						Path         string `yaml:"path"`
						Method       string `yaml:"method"`
						Summary      string `yaml:"summary"`
						AuthRequired *bool  `yaml:"auth_required"`
						Idempotent   *bool  `yaml:"idempotent"`
					} `yaml:"routes"`
				} `yaml:"interface"`
			} `yaml:"agent"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		iface := doc.Agent.Interface
		if iface.Transport != "http" || len(iface.Routes) == 0 {
			continue
		}

		built := render.InterfaceOf{
			AgentID:   doc.Agent.ID,
			Purpose:   doc.Agent.Purpose,
			Transport: iface.Transport,
			BasePath:  iface.BasePath,
			Auth:      iface.Caller.IdentifiedBy,
		}
		for _, r := range iface.Routes {
			// auth_required defaults to true. An endpoint that is open has to say so, because the
			// opposite default means one forgotten field ships an unauthenticated route.
			auth := true
			if r.AuthRequired != nil {
				auth = *r.AuthRequired
			}
			idem := false
			if r.Idempotent != nil {
				idem = *r.Idempotent
			}
			built.Routes = append(built.Routes, render.Route{
				Path:         r.Path,
				Method:       r.Method,
				Summary:      r.Summary,
				AuthRequired: auth,
				Idempotent:   idem,
			})
		}
		out = append(out, built)
	}
	return out, nil
}
