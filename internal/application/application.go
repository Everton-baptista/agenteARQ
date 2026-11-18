// Package application is the shared boundary used by CLI, MCP and the local dashboard.
package application

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentarch "github.com/Everton-baptista/agenteARQ"
	"github.com/Everton-baptista/agenteARQ/internal/policy"
	"github.com/Everton-baptista/agenteARQ/internal/validate"
	"github.com/Everton-baptista/agenteARQ/internal/workflow"
)

type View struct {
	SchemaVersion string             `json:"schema_version"`
	Initialized   bool               `json:"initialized"`
	Root          string             `json:"root"`
	Snapshot      *workflow.Snapshot `json:"snapshot,omitempty"`
	Message       string             `json:"message,omitempty"`
}

func Status(root string) (View, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return View{}, err
	}
	v := View{SchemaVersion: "1.0", Root: abs}
	if _, err := os.Stat(filepath.Join(abs, workflow.ProductPath)); os.IsNotExist(err) {
		v.Message = "Workflow ainda não inicializado. Defina o objetivo e o perfil do produto no seu assistente."
		return v, nil
	}
	s, err := workflow.New(abs)
	if err != nil {
		return v, err
	}
	snap, err := s.Status()
	if err != nil {
		return v, err
	}
	v.Initialized = true
	v.Snapshot = &snap
	return v, nil
}

// Verify uses the same agent gate from every interface, never an LLM verdict.
func Verify(ctx context.Context, root string, opts workflow.VerifyOptions) (workflow.Snapshot, error) {
	s, err := workflow.New(root)
	if err != nil {
		return workflow.Snapshot{}, err
	}
	opts.AgentChecker = CheckAgents
	return s.Verify(ctx, opts)
}

func CheckAgents(ctx context.Context, root string, ids []string) ([]workflow.CheckResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := confineArtifacts(root); err != nil {
		return nil, err
	}
	profile, err := agentProfile(root)
	if err != nil {
		return nil, err
	}
	validator, err := validate.New(agentarch.Spec)
	if err != nil {
		return nil, err
	}
	structural, err := validator.Project(root)
	if err != nil {
		return nil, err
	}
	if len(structural) > 0 {
		out := make([]workflow.CheckResult, 0, len(structural))
		for _, f := range structural {
			out = append(out, workflow.CheckResult{ID: f.ID, Subject: "agents", Status: "failed", Message: f.String(), ExitCode: 2})
		}
		return out, nil
	}
	var content fs.FS
	installed := filepath.Join(root, "agentarch", "std")
	if _, err := os.Stat(filepath.Join(installed, "packs")); err == nil {
		content = os.DirFS(installed)
	} else {
		content, _ = fs.Sub(agentarch.Content, "content")
	}
	cat, err := policy.LoadCatalog(content)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	agents, err := policy.LoadAgents(root, now)
	if err != nil {
		return nil, err
	}
	waivers, err := policy.LoadWaivers(filepath.Join(root, "agentarch", "project", "waivers.yaml"))
	if err != nil {
		return nil, err
	}
	if problems := policy.CheckWaivers(waivers, now); len(problems) > 0 {
		return nil, fmt.Errorf("invalid or expired waivers: %v", problems)
	}
	var out []workflow.CheckResult
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found := false
		for _, a := range agents {
			if a.ID != id {
				continue
			}
			found = true
			node, _ := a.Ctx["agent"].(map[string]any)
			resolved, missing, e := policy.ResolvePacks(cat, policy.PacksFor(profile, node), node)
			if e != nil {
				return nil, e
			}
			if len(missing) > 0 {
				out = append(out, workflow.CheckResult{ID: "agent-packs", Subject: id, Status: "failed", Message: fmt.Sprintf("Required packs missing: %v", missing), ExitCode: 4})
			}
			for _, r := range policy.Evaluate(cat, resolved, a.Ctx, waivers, now) {
				if r.Skipped {
					continue
				}
				status := "passed"
				code := 0
				if r.Error != "" || !r.Passed {
					status = "failed"
					code = 4
				}
				out = append(out, workflow.CheckResult{ID: r.ControlID, Subject: id, Status: status, Message: r.Message, ExitCode: code})
			}
		}
		if !found {
			out = append(out, workflow.CheckResult{ID: "agent-manifest", Subject: id, Status: "failed", Message: "Declared agent manifest not found", ExitCode: 2})
		}
	}
	return out, nil
}

// Match the configured project policy rather than silently downgrading regulated projects.
// A product-only project without agentarch config uses standard for any explicit executors.
func agentProfile(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "agentarch", "agentarch.yaml"))
	if os.IsNotExist(err) {
		return "standard", nil
	}
	if err != nil {
		return "", err
	}
	var cfg struct {
		Project struct {
			DefaultProfile string `yaml:"default_profile"`
		} `yaml:"project"`
	}
	if err = yaml.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid policy configuration: %w", err)
	}
	profile := strings.TrimSpace(cfg.Project.DefaultProfile)
	if profile == "" {
		profile = "minimal"
	}
	if _, ok := policy.Profiles[profile]; !ok {
		return "", fmt.Errorf("unknown configured policy profile %q", profile)
	}
	return profile, nil
}
