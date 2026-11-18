package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var profiles = map[string][]Command{
	"universal": {},
	"typescript-web": {
		{ID: "lint", Program: "npm", Args: []string{"run", "lint"}, Timeout: 2 * time.Minute},
		{ID: "typecheck", Program: "npm", Args: []string{"run", "typecheck"}, Timeout: 2 * time.Minute},
		{ID: "test", Program: "npm", Args: []string{"run", "test", "--", "--run"}, Timeout: 5 * time.Minute},
		{ID: "build", Program: "npm", Args: []string{"run", "build"}, Timeout: 5 * time.Minute},
		{ID: "e2e", Program: "npm", Args: []string{"run", "test:e2e"}, Timeout: 5 * time.Minute},
		{ID: "accessibility", Program: "npm", Args: []string{"run", "test:a11y"}, Timeout: 3 * time.Minute},
		{ID: "security", Program: "npm", Args: []string{"run", "test:security"}, Timeout: 3 * time.Minute},
	},
	"python-api": {
		{ID: "lint", Program: "python", Args: []string{"-m", "ruff", "check", "."}, Timeout: 2 * time.Minute},
		{ID: "typecheck", Program: "python", Args: []string{"-m", "mypy", "."}, Timeout: 2 * time.Minute},
		{ID: "test", Program: "python", Args: []string{"-m", "pytest", "-q"}, Timeout: 5 * time.Minute},
		{ID: "dependencies", Program: "python", Args: []string{"-m", "pip", "check"}, Timeout: time.Minute},
		// Bandit scans '.' as './path'; explicit root globs match that prefix and
		// do not accidentally exclude application paths such as src/tests/.
		{ID: "security", Program: "python", Args: []string{"-m", "bandit", "-r", ".", "-q", "-x", "./.venv/*,./tests/*"}, Timeout: 3 * time.Minute},
	},
	// Go services. Modules come from the image's read-only cache (GOPROXY=off); known
	// vulnerabilities in go.mod are covered by the security floor's dependency-audit.
	"go-service": {
		// gofmt -l exits 0 even when files need formatting, so the fixed wrapper fails on output.
		{ID: "format", Program: "sh", Args: []string{"-c", `out=$(gofmt -l .) || exit 1; [ -z "$out" ] || { printf 'not gofmt-formatted:\n%s\n' "$out"; exit 1; }`}, Timeout: time.Minute},
		{ID: "vet", Program: "go", Args: []string{"vet", "./..."}, Timeout: 3 * time.Minute},
		{ID: "build", Program: "go", Args: []string{"build", "./..."}, Timeout: 5 * time.Minute},
		{ID: "test", Program: "go", Args: []string{"test", "-race", "./..."}, Timeout: 10 * time.Minute},
	},
}

// securityFloor runs in every profile, universal included, after the profile's own checks.
// No product.yaml setting can remove, replace or rename these checks. The programs are wrappers
// installed by verifiers/floor/install.sh; they pin the scanner configuration so a file inside
// the project cannot switch rules off. An image without them exits 127, which the container
// runner records as inconclusive, so a missing floor blocks readiness instead of passing.
var securityFloor = []Command{
	{ID: "secrets", Program: "agentarch-secrets", Timeout: 3 * time.Minute},
	{ID: "dependency-audit", Program: "agentarch-dependency-audit", Timeout: 5 * time.Minute},
}

// FloorCheckIDs are the check IDs every verification must contain.
func FloorCheckIDs() []string {
	ids := make([]string, 0, len(securityFloor))
	for _, c := range securityFloor {
		ids = append(ids, c.ID)
	}
	return ids
}

func Profiles() []string { return []string{"universal", "typescript-web", "python-api", "go-service"} }

// ProfileCommands returns the profile's built-in checks followed by the security floor.
func ProfileCommands(name string) ([]Command, error) {
	p, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown engineering profile %q", name)
	}
	out := append([]Command{}, p...)
	return append(out, securityFloor...), nil
}
func commandsFor(p Product) ([]Command, error) {
	out, e := ProfileCommands(p.Profile)
	if e != nil {
		return nil, e
	}
	if p.Profile != "universal" && len(p.Checks) > 0 {
		return nil, errors.New("custom checks belong to the universal profile; built-in profiles cannot be weakened")
	}
	if p.Profile == "universal" && len(p.Checks) == 0 {
		return nil, errors.New("universal profile requires explicit checks in product.yaml")
	}
	ids := map[string]bool{}
	for _, c := range securityFloor {
		ids[c.ID] = true
	}
	for _, c := range p.Checks {
		for _, f := range securityFloor {
			if c.ID == f.ID {
				return nil, fmt.Errorf("check id %q is reserved for the security floor and cannot be redefined", c.ID)
			}
		}
		if !slug.MatchString(c.ID) || ids[c.ID] {
			return nil, errors.New("custom checks require unique lowercase ids")
		}
		ids[c.ID] = true
		if strings.TrimSpace(c.Program) == "" || strings.ContainsAny(c.Program, "\x00\r\n") {
			return nil, errors.New("check program must be a nonempty executable name")
		}
		if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 600 {
			return nil, errors.New("check timeout_seconds must be between 1 and 600")
		}
		out = append(out, Command{ID: c.ID, Program: c.Program, Args: c.Args, Timeout: time.Duration(c.TimeoutSeconds) * time.Second})
	}
	return out, nil
}

func (s *Service) Verify(ctx context.Context, o VerifyOptions) (Snapshot, error) {
	if err := ValidateDiagnosticsReason(o.DiagnosticsReason); err != nil {
		return Snapshot{}, err
	}
	unlock, e := s.lock()
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	p, t, e := s.loadArtifacts()
	if e != nil {
		return Snapshot{}, e
	}
	st, e := s.loadState()
	if e != nil {
		return Snapshot{}, e
	}
	if e = revisionOK(st, o.ExpectedRevision); e != nil {
		return Snapshot{}, e
	}
	if st.Phase != "implementing" && st.Phase != "verifying" && st.Phase != "reviewing" && st.Phase != "pr_ready" {
		return Snapshot{}, errors.New("implementation and task checkpoints are required before verification")
	}
	if missing := s.requirements(p, t.Tasks, "planned"); len(missing) > 0 {
		return Snapshot{}, fmt.Errorf("verification prerequisites: %s", strings.Join(missing, "; "))
	}
	for _, task := range t.Tasks {
		if !contains(st.CompletedTasks, task.ID) {
			return Snapshot{}, fmt.Errorf("task %s is incomplete", task.ID)
		}
	}
	commands, e := commandsFor(p)
	if e != nil {
		return Snapshot{}, e
	}
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	runner := o.Runner
	if runner == nil {
		runner = &ContainerRunner{Image: o.Image, DiagnosticsReason: o.DiagnosticsReason}
	}
	v := &Verification{Fingerprint: fp, StartedAt: now(), Isolation: runner.Isolation(), Checks: []CheckResult{}, DiagnosticsReason: strings.TrimSpace(o.DiagnosticsReason)}
	// A single bounded verification run. Hosts cannot use this endpoint to start a model.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	for _, c := range commands {
		if ctx.Err() != nil {
			v.Checks = append(v.Checks, CheckResult{ID: c.ID, Subject: "product", Status: "inconclusive", Message: "Verification was cancelled or exceeded its time budget.", ExitCode: -1})
			continue
		}
		check := runner.Run(ctx, s.Root, c)
		check.ID = c.ID
		check.Subject = "product"
		if check.Status != "passed" && check.Status != "failed" && check.Status != "inconclusive" {
			check.Status = "inconclusive"
			check.Message = "Verifier returned an unsupported result."
		}
		if check.Status == "passed" && check.ExitCode != 0 {
			check.Status = "failed"
			check.Message = "A nonzero exit cannot pass verification."
		}
		v.Checks = append(v.Checks, check)
	}
	for _, group := range []struct {
		subject string
		ids     []string
	}{{"executor", p.ExecutorIDs}, {"application_agent", p.ApplicationAgentIDs}} {
		if len(group.ids) == 0 {
			continue
		}
		if o.AgentChecker == nil {
			v.Checks = append(v.Checks, CheckResult{ID: "ai.gate", Subject: group.subject, Status: "inconclusive", Message: "AI gate adapter is unavailable for the declared agents.", ExitCode: -1})
			continue
		}
		checks, err := o.AgentChecker(ctx, s.Root, group.ids)
		if err != nil || len(checks) == 0 {
			v.Checks = append(v.Checks, CheckResult{ID: "ai.gate", Subject: group.subject, Status: "inconclusive", Message: "AI gate did not return verifiable results for the declared agents.", ExitCode: -1})
			continue
		}
		for _, c := range checks {
			if c.AgentID == "" && c.Subject != "agents" && c.Subject != group.subject {
				c.AgentID = c.Subject
			}
			c.Subject = group.subject
			if c.Status != "passed" && c.Status != "failed" {
				c.Status = "inconclusive"
			}
			v.Checks = append(v.Checks, c)
		}
	}
	after, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	if !sameFingerprint(fp, after) {
		v.Checks = append(v.Checks, CheckResult{ID: "source.changed", Subject: "product", Status: "inconclusive", Message: "Source changed during verification; repeat on a stable worktree.", ExitCode: -1})
	}
	v.FinishedAt = now()
	v.Passed = true
	for _, c := range v.Checks {
		if c.Status != "passed" {
			v.Passed = false
		}
	}
	st.Verification = v
	st.Review = nil
	st.Phase = "verifying"
	if v.Passed {
		st.Phase = "reviewing"
	}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}
