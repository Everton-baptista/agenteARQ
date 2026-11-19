package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

type fakeRunner struct {
	status    string
	statusFor map[string]string // per check ID; overrides status
	after     func()
	once      sync.Once
}

func (f *fakeRunner) Isolation() string { return "simulated" }
func (f *fakeRunner) Run(_ context.Context, _ string, c Command) CheckResult {
	if f.after != nil {
		f.once.Do(f.after)
	}
	status := f.status
	if s, ok := f.statusFor[c.ID]; ok {
		status = s
	}
	if status == "" {
		status = "passed"
	}
	code := 0
	if status != "passed" {
		code = 1
	}
	return CheckResult{ID: c.ID, Status: status, ExitCode: code, Message: "Deterministic infrastructure fixture; no model was called."}
}
func write(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", root)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("git init: %s %v", b, e)
	}
	write(t, root, "README.md", "Product fixture\n")
	return root
}
func fixture(t *testing.T) (*Service, Snapshot) {
	t.Helper()
	root := repo(t)
	s, e := New(root)
	if e != nil {
		t.Fatal(e)
	}
	snap, e := s.Init(InitOptions{Name: "Example product", Profile: "typescript-web", Goal: "Let customers find their own orders", Author: "implementer"})
	if e != nil {
		t.Fatal(e)
	}
	return s, snap
}
func prepare(t *testing.T, s *Service) {
	t.Helper()
	for _, rel := range documents {
		write(t, s.Root, rel, "# Reviewed product artifact\n\nAC-1: Customer can find only their own orders.\nDecision: scoped API with independent access checks.\n")
	}
	td := TaskDocument{SchemaVersion: SchemaVersion, Tasks: []Task{{ID: "order-search", Title: "Implement tenant-scoped order search", Acceptance: []string{"AC-1"}, Checks: []string{"test", "security"}}}}
	b, e := yaml.Marshal(td)
	if e != nil {
		t.Fatal(e)
	}
	write(t, s.Root, TasksPath, string(b))
	for _, phase := range []string{"specified", "planned", "implementing"} {
		if _, e = s.Checkpoint(CheckpointRequest{Phase: phase}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.Checkpoint(CheckpointRequest{Phase: "implementing", TaskID: "order-search"}); e != nil {
		t.Fatal(e)
	}
	commit(t, s)
}
func commit(t *testing.T, s *Service) {
	t.Helper()
	for _, args := range [][]string{{"add", "--all"}, {"-c", "user.name=Workflow Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Product checkpoint"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = s.Root
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("commit: %s %v", out, e)
		}
	}
}

func verify(t *testing.T, s *Service) Snapshot {
	t.Helper()
	snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{}})
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.Phase != "reviewing" {
		t.Fatalf("wanted reviewing: %+v", snap)
	}
	return snap
}
func reviewReq() ReviewRequest {
	return ReviewRequest{Reviewer: "independent-reviewer", Kind: "human", Verdict: "approved", Summary: "Acceptance mapping and access boundaries reviewed independently."}
}

// This is a deterministic workflow benchmark, not a model benchmark. Every runner is
// explicitly marked simulated; no pass rate here claims improved generated-code quality.
func TestBenchmarkTwelveScenarios(t *testing.T) {
	scenarios := []struct {
		name string
		run  func(*testing.T, *Service, Snapshot)
	}{
		{"01_ordinary_non_ai_product", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			called := false
			snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{}, AgentChecker: func(context.Context, string, []string) ([]CheckResult, error) { called = true; return nil, nil }})
			if e != nil {
				t.Fatal(e)
			}
			if called || !snap.State.Verification.Passed {
				t.Fatal("ordinary application incorrectly needs AI controls")
			}
			snap, e = s.Review(reviewReq())
			if e != nil || snap.State.Phase != "pr_ready" {
				t.Fatalf("not ready: %v %+v", e, snap)
			}
		}},
		{"02_executor_checked_without_product_ai", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			p, _, _ := s.loadArtifacts()
			p.ExecutorIDs = []string{"builder"}
			b, _ := yaml.Marshal(p)
			write(t, s.Root, ProductPath, string(b))
			calls := 0
			snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{}, AgentChecker: func(_ context.Context, _ string, ids []string) ([]CheckResult, error) {
				calls++
				if len(ids) != 1 || ids[0] != "builder" {
					t.Fatal(ids)
				}
				return []CheckResult{{ID: "manifest", Status: "passed"}}, nil
			}})
			if e != nil || calls != 1 || !snap.State.Verification.Passed {
				t.Fatalf("executor gate: %v %+v", e, snap)
			}
			last := snap.State.Verification.Checks[len(snap.State.Verification.Checks)-1]
			if last.Subject != "executor" {
				t.Fatal(last)
			}
		}},
		{"03_missing_isolation_is_inconclusive", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			snap, e := s.Verify(context.Background(), VerifyOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if snap.State.Verification.Passed || snap.State.Phase != "verifying" {
				t.Fatal("missing OCI environment passed")
			}
			for _, c := range snap.State.Verification.Checks {
				if c.Status != "inconclusive" {
					t.Fatal(c)
				}
			}
		}},
		{"04_source_change_invalidates_review", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			verify(t, s)
			if _, e := s.Review(reviewReq()); e != nil {
				t.Fatal(e)
			}
			write(t, s.Root, "README.md", "Changed source after review\n")
			snap, e := s.Status()
			if e != nil {
				t.Fatal(e)
			}
			if snap.State.Phase != "verifying" || len(snap.Findings) == 0 {
				t.Fatal("stale evidence stayed ready")
			}
			if _, e = s.Review(reviewReq()); e == nil {
				t.Fatal("stale review accepted")
			}
		}},
		{"05_edit_during_verification", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{after: func() { write(t, s.Root, "README.md", "edited during verification") }}})
			if e != nil {
				t.Fatal(e)
			}
			if snap.State.Verification.Passed {
				t.Fatal("concurrent edit passed")
			}
		}},
		{"06_task_dependencies_are_ordered", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			_, tasks, _ := s.loadArtifacts()
			tasks.Tasks = append(tasks.Tasks, Task{ID: "customer-ui", Title: "Implement customer search interface", Acceptance: []string{"AC-1"}, Checks: []string{"test"}, DependsOn: []string{"new-api"}}, Task{ID: "new-api", Title: "Implement customer search endpoint", Acceptance: []string{"AC-1"}, Checks: []string{"test"}})
			b, _ := yaml.Marshal(tasks)
			write(t, s.Root, TasksPath, string(b))
			if _, e := s.Checkpoint(CheckpointRequest{TaskID: "customer-ui"}); e == nil {
				t.Fatal("dependency skipped")
			}
			if _, e := s.Checkpoint(CheckpointRequest{TaskID: "new-api"}); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Checkpoint(CheckpointRequest{TaskID: "customer-ui"}); e != nil {
				t.Fatal(e)
			}
		}},
		{"07_quality_phases_cannot_be_claimed", func(t *testing.T, s *Service, _ Snapshot) {
			for _, p := range []string{"planned", "reviewing", "pr_ready"} {
				if _, e := s.Checkpoint(CheckpointRequest{Phase: p}); e == nil {
					t.Fatalf("accepted %s", p)
				}
			}
		}},
		{"08_self_review_rejected", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			verify(t, s)
			r := reviewReq()
			r.Reviewer = " IMPLEMENTER "
			if _, e := s.Review(r); e == nil {
				t.Fatal("self review accepted")
			}
		}},
		{"09_concurrent_writer_rejected", func(t *testing.T, s *Service, _ Snapshot) {
			release, e := s.lock()
			if e != nil {
				t.Fatal(e)
			}
			defer release()
			other, e := New(s.Root)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = other.Checkpoint(CheckpointRequest{Phase: "blocked", Note: "Waiting for required user input"}); e == nil {
				t.Fatal("concurrent writer accepted")
			}
		}},
		{"10_stale_revision_rejected", func(t *testing.T, s *Service, snap Snapshot) {
			old := snap.State.Revision
			if _, e := s.Checkpoint(CheckpointRequest{Phase: "blocked", Note: "Waiting for product requirements"}); e != nil {
				t.Fatal(e)
			}
			if _, e := s.Checkpoint(CheckpointRequest{Phase: "draft", ExpectedRevision: &old}); e == nil {
				t.Fatal("stale revision accepted")
			}
		}},
		{"11_host_restart_preserves_state", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			snap := verify(t, s)
			other, e := New(s.Root)
			if e != nil {
				t.Fatal(e)
			}
			after, e := other.Status()
			if e != nil {
				t.Fatal(e)
			}
			if after.State.Revision != snap.State.Revision || after.State.Phase != "reviewing" {
				t.Fatal("restart lost state")
			}
		}},
		{"12_changes_requested_return_to_implementation", func(t *testing.T, s *Service, _ Snapshot) {
			prepare(t, s)
			verify(t, s)
			r := reviewReq()
			r.Verdict = "changes_requested"
			snap, e := s.Review(r)
			if e != nil {
				t.Fatal(e)
			}
			if snap.State.Phase != "implementing" {
				t.Fatal("requested changes did not block readiness")
			}
		}},
	}
	if len(scenarios) != 12 {
		t.Fatal("benchmark must document its twelve scenarios")
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) { s, snap := fixture(t); sc.run(t, s, snap) })
	}
}

func TestTaskCyclesAndUnmappedCriteria(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	if e := validateTasks([]Task{{ID: "first", DependsOn: []string{"second"}}, {ID: "second", DependsOn: []string{"first"}}}); e == nil {
		t.Fatal("cycle accepted")
	}
	p, tasks, _ := s.loadArtifacts()
	tasks.Tasks[0].Acceptance = []string{"AC-99"}
	if len(s.requirements(p, tasks.Tasks, "planned")) == 0 {
		t.Fatal("unmapped criterion accepted")
	}
}
func TestMissingAgentCheckerCannotPass(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	p, _, _ := s.loadArtifacts()
	p.ExecutorIDs = []string{"builder"}
	b, _ := yaml.Marshal(p)
	write(t, s.Root, ProductPath, string(b))
	snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{}})
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.Verification.Passed {
		t.Fatal("unverified executor passed")
	}
}
func TestFailureDoesNotAdvance(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	snap, e := s.Verify(context.Background(), VerifyOptions{Runner: &fakeRunner{status: "failed"}})
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.Phase != "verifying" {
		t.Fatal("failure advanced")
	}
}
func TestProductAgentContradiction(t *testing.T) {
	s, _ := fixture(t)
	p, _, _ := s.loadArtifacts()
	p.ApplicationAgentIDs = []string{"assistant"}
	if len(s.requirements(p, nil, "specified")) == 0 {
		t.Fatal("non-AI product with app agent accepted")
	}
}
func TestInitDoesNotOverwrite(t *testing.T) {
	s, _ := fixture(t)
	before, e := os.ReadFile(filepath.Join(s.Root, ProductPath))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Init(InitOptions{Name: "Replacement"}); e == nil {
		t.Fatal("second init succeeded")
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, ProductPath))
	if string(before) != string(after) {
		t.Fatal("init overwrote artifacts")
	}
}
func TestStrictYAMLAndSafeArtifactPath(t *testing.T) {
	s, _ := fixture(t)
	b, _ := os.ReadFile(filepath.Join(s.Root, ProductPath))
	write(t, s.Root, ProductPath, string(b)+"unknown_authority: admin\n")
	if _, e := s.Status(); e == nil {
		t.Fatal("unknown YAML key accepted")
	}
	if _, e := safePath(s.Root, "../../outside"); e == nil {
		t.Fatal("path escape accepted")
	}
}
func TestUniversalChecksAndProfiles(t *testing.T) {
	cmds, e := commandsFor(Product{Profile: "universal", Checks: []CheckSpec{{ID: "unit", Program: "cargo", Args: []string{"test", "--offline"}, TimeoutSeconds: 60}}})
	if e != nil || len(cmds) != 1+len(securityFloor) || cmds[len(cmds)-1].ID != "unit" {
		t.Fatalf("universal: %+v, %v", cmds, e)
	}
	if _, e = commandsFor(Product{Profile: "universal"}); e == nil {
		t.Fatal("empty universal profile passed")
	}
	if _, e = commandsFor(Product{Profile: "python-api", Checks: []CheckSpec{{ID: "unit"}}}); e == nil {
		t.Fatal("built-in profile weakened")
	}
}
func TestFingerprintIgnoresLocalStateButTracksModeAndDelete(t *testing.T) {
	s, _ := fixture(t)
	a, e := s.fingerprint("typescript-web")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Checkpoint(CheckpointRequest{Phase: "blocked", Note: "Needs clarification"}); e != nil {
		t.Fatal(e)
	}
	b, _ := s.fingerprint("typescript-web")
	if a != b {
		t.Fatal("local metadata changed source hash")
	}
	if e = os.Chmod(filepath.Join(s.Root, "README.md"), 0755); e != nil {
		t.Fatal(e)
	}
	c, _ := s.fingerprint("typescript-web")
	if a == c {
		t.Fatal("executable mode not hashed")
	}
}
func TestContainerHasNoLocalFallback(t *testing.T) {
	r := &ContainerRunner{Image: "python:latest"}
	c := Command{ID: "test", Program: "python", Args: []string{"-c", "raise SystemExit(0)"}}
	out := r.Run(context.Background(), t.TempDir(), c)
	if out.Status != "inconclusive" || !strings.Contains(out.Message, "digest") {
		t.Fatal(out)
	}
}
func TestSourceStagingRejectsEscapingSymlink(t *testing.T) {
	s, _ := fixture(t)
	if e := os.Symlink("/etc/passwd", filepath.Join(s.Root, "external")); e != nil {
		t.Skip(e)
	}
	if e := s.copySource(t.TempDir()); e == nil {
		t.Fatal("external symlink accepted")
	}
}
func TestSourceStagingOmitsGitAndIgnoredSecrets(t *testing.T) {
	s, _ := fixture(t)
	write(t, s.Root, ".gitignore", ".env\n")
	write(t, s.Root, ".env", "TEST_DUMMY_VALUE=not-a-real-secret\n")
	dest := t.TempDir()
	if e := s.copySource(dest); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{".git", ".env"} {
		if _, e := os.Stat(filepath.Join(dest, name)); !os.IsNotExist(e) {
			t.Fatalf("staged %s", name)
		}
	}
}

func TestReviewRequiresCommittedSource(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	write(t, s.Root, "README.md", "Uncommitted implementation change\n")
	verify(t, s)
	if _, e := s.Review(reviewReq()); e == nil {
		t.Fatal("approved uncommitted source")
	}
}
func TestRecordPRRequiresOriginAndExactReviewedHead(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	verify(t, s)
	snap, e := s.Review(reviewReq())
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:example/product.git")
	cmd.Dir = s.Root
	if e = cmd.Run(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RecordPR(PRRequest{URL: "https://evil.example/example/product/pull/1", Head: snap.Fingerprint.Head}); e == nil {
		t.Fatal("foreign host accepted")
	}
	if _, e = s.RecordPR(PRRequest{URL: "https://github.com/example/product/pull/1", Head: "wrong"}); e == nil {
		t.Fatal("wrong commit accepted")
	}
	snap, e = s.RecordPR(PRRequest{URL: "https://github.com/example/product/pull/1", Head: snap.Fingerprint.Head})
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.PR == nil || snap.State.PR.URL == "" {
		t.Fatal("PR record missing")
	}
}
func TestEvidenceExportDoesNotInvalidateItself(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	snap := verify(t, s)
	if _, e := os.Stat(filepath.Join(s.Root, EvidencePath)); e != nil {
		t.Fatal(e)
	}
	after, e := s.Status()
	if e != nil {
		t.Fatal(e)
	}
	if after.Fingerprint != snap.State.Verification.Fingerprint {
		t.Fatal("export invalidated source")
	}
	write(t, s.Root, EvidencePath, `{"phase":"pr_ready"}`)
	after, e = s.Status()
	if e != nil {
		t.Fatal(e)
	}
	if after.State.Phase != "reviewing" {
		t.Fatal("edited export changed authoritative state")
	}
}
func TestContainerArgumentsKeepCodeIsolated(t *testing.T) {
	injected := "$(touch /outside)"
	args := containerArgs("example/verifier@sha256:"+strings.Repeat("a", 64), "/tmp/stage", "/tmp/cid", "fixture-root", Command{Program: "npm", Args: []string{"run", injected}})
	joined := strings.Join(args, "\n")
	for _, flag := range []string{"--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--cpus=2", "--interactive", "/workspace:rw,nosuid,nodev,size=512m"} {
		if !strings.Contains(joined, flag) {
			t.Fatalf("missing containment %s", flag)
		}
	}
	if strings.Contains(joined, "--mount") || strings.Contains(joined, "/tmp/stage") {
		t.Fatal("host directory exposed to daemon")
	}
	if args[len(args)-1] != injected {
		t.Fatal("arguments were interpolated")
	}
	shell := ""
	for i, a := range args {
		if a == "-c" {
			shell = args[i+1]
			break
		}
	}
	if shell == "" || strings.Contains(shell, injected) {
		t.Fatal("untrusted value entered shell code")
	}
	if imageDigest.MatchString("--privileged@sha256:" + strings.Repeat("a", 64)) {
		t.Fatal("image can inject an option")
	}
}
