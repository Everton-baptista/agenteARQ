package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func acceptReq(verdict string) AcceptRequest {
	return AcceptRequest{Owner: "Product Owner", Verdict: verdict, Summary: "Searched my own orders on the phone; the list matched what I expected."}
}

func TestOwnerAcceptanceIsAdvisedRecordedAndExported(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	if _, e := s.Accept(acceptReq("accepted")); e == nil {
		t.Fatal("acceptance recorded before verification")
	}
	verify(t, s)
	if _, e := s.Review(reviewReq()); e != nil {
		t.Fatal(e)
	}
	n, _ := s.Next()
	if !hasAdvice(n, AdviceAccept) {
		t.Fatalf("pr_ready without acceptance gives no advice: %v", n.Advice)
	}
	snap, e := s.Accept(acceptReq("accepted"))
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.Phase != "pr_ready" || snap.State.OwnerAcceptance == nil || hasAdvice(snap.Next, AdviceAccept) {
		t.Fatalf("acceptance not recorded: %+v", snap.State)
	}
	b, e := os.ReadFile(filepath.Join(s.Root, EvidencePath))
	if e != nil {
		t.Fatal(e)
	}
	var ev Evidence
	if e = json.Unmarshal(b, &ev); e != nil || ev.OwnerAcceptance == nil || ev.OwnerAcceptance.Verdict != "accepted" {
		t.Fatalf("acceptance not exported: %s %v", b, e)
	}
	// Changing the content after acceptance means asking the owner again.
	write(t, s.Root, "README.md", "Product fixture, changed after acceptance\n")
	snap, _ = s.Status()
	if snap.State.OwnerAcceptance != nil {
		t.Fatal("stale acceptance still reported")
	}
}

func TestOwnerRejectionReturnsToImplementing(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	verify(t, s)
	snap, e := s.Accept(acceptReq("rejected"))
	if e != nil {
		t.Fatal(e)
	}
	if snap.State.Phase != "implementing" || snap.State.Review != nil {
		t.Fatalf("rejection did not reopen implementation: %+v", snap.State)
	}
	found := false
	for _, f := range snap.Findings {
		if f.ID == "owner.rejected" && strings.Contains(f.Message, "the list matched") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the owner's words are not kept as a finding: %+v", snap.Findings)
	}
	if _, e = s.Checkpoint(CheckpointRequest{Phase: "verifying"}); e != nil {
		t.Fatal(e)
	}
	snap, _ = s.Status()
	if snap.State.OwnerAcceptance != nil {
		t.Fatal("rejection survived moving on to verifying")
	}
}

func TestAcceptanceValidation(t *testing.T) {
	s, _ := fixture(t)
	prepare(t, s)
	verify(t, s)
	for _, c := range []struct {
		req  AcceptRequest
		want string
	}{
		{AcceptRequest{Owner: "", Verdict: "accepted", Summary: "Tried every journey, all fine."}, "owner must name"},
		{AcceptRequest{Owner: "Owner", Verdict: "maybe", Summary: "Tried every journey, all fine."}, "accepted or rejected"},
		{AcceptRequest{Owner: "Owner", Verdict: "accepted", Summary: "TODO"}, "summary"},
	} {
		if _, e := s.Accept(c.req); e == nil || !strings.Contains(e.Error(), c.want) {
			t.Errorf("%+v: got %v, want %q", c.req, e, c.want)
		}
	}
}
