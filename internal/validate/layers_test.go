package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// project writes a tiny tree and returns its root.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var serviceLayout = Layout{
	Edge:   []string{"app/api/**"},
	Core:   []string{"app/agent/**"},
	Client: []string{"web/**"},
}

func TestLayersAcceptsACoreThatOnlyImportsDownward(t *testing.T) {
	root := project(t, map[string]string{
		"app/agent/runner.py": "" +
			"from ..domain import orders\n" +
			"from ..infra.provider import create_message\n" +
			"from .guardrails import input_guardrail\n",
		"app/api/routes.py": "from ..agent import runner\n",
	})
	if got := LintLayers(root, serviceLayout); len(got) != 0 {
		t.Errorf("expected no findings, got %v", got)
	}
}

// The mistake people actually make: a handler needs one header, so the loop imports the request.
func TestLayersRejectsCoreImportingTheEdge(t *testing.T) {
	root := project(t, map[string]string{
		"app/agent/runner.py": "from ..api.deps import current_principal\n",
	})
	got := LintLayers(root, serviceLayout)
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %v", got)
	}
	if got[0].ID != "AA-DEP-019" {
		t.Errorf("id = %s", got[0].ID)
	}
	if !strings.Contains(got[0].Pointer, "line 1") {
		t.Errorf("pointer should name the line, got %q", got[0].Pointer)
	}
}

// Importing the framework directly is the same problem by another route: either way the agent can
// only run inside a server.
func TestLayersRejectsCoreImportingTheWebFramework(t *testing.T) {
	for _, line := range []string{
		"from fastapi import Depends\n",
		"import flask\n",
		"from starlette.requests import Request\n",
		"import net/http\n",
		"using Microsoft.AspNetCore.Http;\n",
	} {
		root := project(t, map[string]string{"app/agent/runner.py": line})
		if got := LintLayers(root, serviceLayout); len(got) == 0 {
			t.Errorf("%q was not reported", strings.TrimSpace(line))
		}
	}
}

func TestLayersIgnoresNonImportLines(t *testing.T) {
	// A comment or a docstring mentioning the transport is not a dependency on it — and these files
	// explain themselves at length, so a substring match would fail on the prose that exists to
	// prevent the very thing it describes.
	root := project(t, map[string]string{
		"app/agent/runner.py": "" +
			`"""This module must never import from app.api — see AA-DEP-019."""` + "\n" +
			"# fastapi is deliberately absent here\n" +
			"MESSAGE = 'do not import ..api'\n",
	})
	if got := LintLayers(root, serviceLayout); len(got) != 0 {
		t.Errorf("prose was reported as a dependency: %v", got)
	}
}

func TestLayersSaysNothingWhenNoLayoutIsDeclared(t *testing.T) {
	// The control reports the missing declaration. Checking an undeclared layout would mean checking
	// one the project never agreed to.
	root := project(t, map[string]string{"app/agent/runner.py": "from ..api.deps import x\n"})
	if got := LintLayers(root, Layout{}); len(got) != 0 {
		t.Errorf("expected silence with no layout, got %v", got)
	}
}

func TestLayersWorksWithADifferentButEquallyValidLayout(t *testing.T) {
	// The rule is the direction of the arrows, not the spelling of the directories. A project using
	// src/http and src/brain must be checkable without renaming anything.
	layout := Layout{Edge: []string{"src/http/**"}, Core: []string{"src/brain/**"}}
	root := project(t, map[string]string{
		"src/brain/loop.ts": "import { router } from '../http/routes';\n",
	})
	if got := LintLayers(root, layout); len(got) != 1 {
		t.Fatalf("expected one finding, got %v", got)
	}
}

func TestLayersRejectsAProviderCredentialInTheClient(t *testing.T) {
	root := project(t, map[string]string{
		"web/app.ts": "import Anthropic from '@anthropic-ai/sdk';\n" +
			"const client = new Anthropic({ apiKey: process.env.ANTHROPIC_API_KEY });\n",
	})
	got := LintLayers(root, serviceLayout)
	if len(got) == 0 {
		t.Fatal("a provider SDK in the client was not reported")
	}
	// A client that calls a provider directly has no input check, no output check, no tool
	// authorisation, no budget and no audit trail — so the fix must say to route it, not to hide it.
	if !strings.Contains(got[0].Fix, "your own endpoint") {
		t.Errorf("the fix should name the remedy, got %q", got[0].Fix)
	}
}

func TestLayersAcceptsAClientThatCallsItsOwnBackend(t *testing.T) {
	root := project(t, map[string]string{
		"web/app.ts": "const r = await fetch('/v1/ask', { method: 'POST', body });\n",
	})
	if got := LintLayers(root, serviceLayout); len(got) != 0 {
		t.Errorf("expected no findings, got %v", got)
	}
}

func TestModulePrefixesHandlesTheShapesWeShip(t *testing.T) {
	got := modulePrefixes([]string{"app/api/**", "src/http/*", "/edge/"})
	for _, want := range []string{"app.api", "api", "src.http", "http", "edge"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q missing from %v", want, got)
		}
	}
}
