package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fullReleasePlan = `# Release plan

## Environment
Production on the managed container platform, region eu-west.

## Owner
Ana Silva is accountable for this release and its rollback.

## Rollback
Redeploy the previous image digest from the deployment history, then confirm health.

## Post-deploy verification
Run the order search smoke suite against production and compare error rates.

## Signals
Watch the 5xx rate, p95 latency of order search and authorization failures for one hour.
`

func git_(t *testing.T, s *Service, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Workflow Fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
	cmd.Dir = s.Root
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("git %v: %s %v", args, out, e)
	}
	return strings.TrimSpace(string(out))
}

// readyForRelease drives a product to pr_ready with the given release plan in the reviewed source.
func readyForRelease(t *testing.T, plan string) *Service {
	t.Helper()
	s, _ := fixture(t)
	if plan != "" {
		write(t, s.Root, ReleasePlanPath, plan)
	}
	prepare(t, s)
	verify(t, s)
	if snap, e := s.Review(reviewReq()); e != nil || snap.State.Phase != "pr_ready" {
		t.Fatalf("not pr_ready: %v", e)
	}
	return s
}

func TestReleaseToProductionVerified(t *testing.T) {
	s := readyForRelease(t, fullReleasePlan)

	// A squash merge carries the reviewed tree in a new commit: still pr_ready.
	git_(t, s, "commit", "--allow-empty", "-qm", "Squash merge of the reviewed change")
	head := git_(t, s, "rev-parse", "HEAD")
	snap, e := s.Status()
	if e != nil || snap.State.Phase != "pr_ready" {
		t.Fatalf("content-identical merge demoted the work: %v %s", e, snap.State.Phase)
	}

	if _, e = s.Checkpoint(CheckpointRequest{Phase: "released"}); e == nil {
		t.Fatal("a checkpoint claimed released")
	}
	for _, bad := range []ReleaseRequest{
		{URL: "https://ci.example.com/runs/1", Head: "wrong", Environment: "production"},
		{URL: "http://ci.example.com/runs/1", Head: head, Environment: "production"},
		{URL: "https://user:pw@ci.example.com/runs/1", Head: head, Environment: "production"},
		{URL: "https://ci.example.com/runs/1", Head: head, Environment: "Prod Env"},
	} {
		if _, e = s.RecordRelease(bad); e == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	snap, e = s.RecordRelease(ReleaseRequest{URL: "https://ci.example.com/runs/1", Head: head, Environment: "production"})
	if e != nil || snap.State.Phase != "released" || snap.State.Release == nil || snap.State.Release.Head != head {
		t.Fatalf("release not recorded: %v %+v", e, snap.State.Release)
	}

	for _, bad := range []ProductionCheckRequest{
		{URL: "https://grafana.example.com/d/1", Outcome: "maybe", Attester: "ops", Summary: "Smoke suite and error rates checked."},
		{URL: "https://grafana.example.com/d/1", Outcome: "passed", Attester: " ", Summary: "Smoke suite and error rates checked."},
		{URL: "https://grafana.example.com/d/1", Outcome: "passed", Attester: "ops", Summary: "TODO"},
	} {
		if _, e = s.RecordProductionCheck(bad); e == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	snap, e = s.RecordProductionCheck(ProductionCheckRequest{URL: "https://grafana.example.com/d/1", Outcome: "failed", Attester: "Ana Silva", Summary: "5xx rate doubled after the deploy; rolling back."})
	if e != nil || snap.State.Phase != "released" {
		t.Fatalf("failed check: %v %s", e, snap.State.Phase)
	}
	found := false
	for _, f := range snap.Findings {
		found = found || f.ID == "production.check_failed"
	}
	if !found {
		t.Fatal("a failed production check raised no finding")
	}
	snap, e = s.RecordProductionCheck(ProductionCheckRequest{URL: "https://grafana.example.com/d/2", Outcome: "passed", Attester: "Ana Silva", Summary: "Smoke suite passed; error rate and latency normal for an hour."})
	if e != nil || snap.State.Phase != "production_verified" {
		t.Fatalf("passed check: %v %s", e, snap.State.Phase)
	}
	b, e := os.ReadFile(filepath.Join(s.Root, EvidencePath))
	if e != nil || !strings.Contains(string(b), `"production_check"`) || !strings.Contains(string(b), `"release"`) {
		t.Fatalf("evidence export lacks the release records: %v", e)
	}

	// Shipping new content is a new cycle: the old production evidence no longer applies.
	write(t, s.Root, "src/new-feature.ts", "export const feature = true;\n")
	git_(t, s, "add", "--all")
	git_(t, s, "commit", "-qm", "New feature")
	if snap, _ = s.Status(); snap.State.Phase != "verifying" {
		t.Fatalf("changed content kept phase %s", snap.State.Phase)
	}
}

func TestReleaseRequiresTheReviewedContentAndAPlan(t *testing.T) {
	s := readyForRelease(t, "")
	head := git_(t, s, "rev-parse", "HEAD")
	_, e := s.RecordRelease(ReleaseRequest{URL: "https://ci.example.com/runs/1", Head: head, Environment: "production"})
	if e == nil || !strings.Contains(e.Error(), ReleasePlanPath) {
		t.Fatalf("missing plan: %v", e)
	}

	s = readyForRelease(t, strings.Replace(fullReleasePlan, "Redeploy the previous image digest from the deployment history, then confirm health.", "TODO", 1))
	head = git_(t, s, "rev-parse", "HEAD")
	if _, e = s.RecordRelease(ReleaseRequest{URL: "https://ci.example.com/runs/1", Head: head, Environment: "production"}); e == nil || !strings.Contains(e.Error(), "Rollback") {
		t.Fatalf("empty rollback accepted: %v", e)
	}

	// Content merged alongside the reviewed change is not what was reviewed.
	s = readyForRelease(t, fullReleasePlan)
	write(t, s.Root, "src/extra.ts", "export const unreviewed = 1;\n")
	git_(t, s, "add", "--all")
	git_(t, s, "commit", "-qm", "Merge with an unreviewed change")
	head = git_(t, s, "rev-parse", "HEAD")
	if snap, _ := s.Status(); snap.State.Phase != "verifying" {
		t.Fatalf("unreviewed content kept phase %s", snap.State.Phase)
	}
	if _, e = s.RecordRelease(ReleaseRequest{URL: "https://ci.example.com/runs/1", Head: head, Environment: "production"}); e == nil {
		t.Fatal("released unreviewed content")
	}
	if _, e = s.RecordProductionCheck(ProductionCheckRequest{URL: "https://grafana.example.com/d/1", Outcome: "passed", Attester: "ops", Summary: "Checked production dashboards."}); e == nil {
		t.Fatal("production check without a release")
	}
}
