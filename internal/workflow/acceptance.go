package workflow

import (
	"errors"
	"strings"
)

// Accept records the owner trying the product. Checks prove the product does what the
// specification says; only the owner can say whether the specification said what they meant.
// The record is bound to the verified content, like a review, so changing the product after the
// owner accepted it means asking again.
//
// A rejection is not a failure of the gate but new information: the work returns to
// implementing, and the owner's own words stay visible as the requirement to address.
func (s *Service) Accept(req AcceptRequest) (Snapshot, error) {
	owner := strings.TrimSpace(req.Owner)
	if owner == "" || len(owner) > 120 {
		return Snapshot{}, errors.New("owner must name who tried the product")
	}
	if req.Verdict != "accepted" && req.Verdict != "rejected" {
		return Snapshot{}, errors.New("acceptance verdict must be accepted or rejected")
	}
	if !meaningful(req.Summary) || len(req.Summary) > 2000 {
		return Snapshot{}, errors.New("acceptance summary must say what the owner tried and saw, without TODO")
	}
	unlock, e := s.lock()
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	p, _, e := s.loadArtifacts()
	if e != nil {
		return Snapshot{}, e
	}
	st, e := s.loadState()
	if e != nil {
		return Snapshot{}, e
	}
	if e = revisionOK(st, req.ExpectedRevision); e != nil {
		return Snapshot{}, e
	}
	if st.Phase != "reviewing" && st.Phase != "pr_ready" {
		return Snapshot{}, errors.New("owner acceptance is recorded after verification passes, in reviewing or pr_ready")
	}
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	if st.Verification == nil || !st.Verification.Passed || !acceptanceCurrent(st.Phase, st.Verification.Fingerprint, fp) {
		return Snapshot{}, errors.New("verification evidence is stale or failed; verify again")
	}
	st.OwnerAcceptance = &AcceptanceRecord{Owner: owner, Verdict: req.Verdict, Summary: strings.TrimSpace(req.Summary), Fingerprint: fp, At: now()}
	if req.Verdict == "rejected" {
		st.Phase = "implementing"
		st.Review = nil
		st.PR, st.Release, st.ProductionCheck = nil, nil, nil
	}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}

// acceptanceCurrent mirrors how status judges verification: from pr_ready on only a change of
// content matters, before it any change of commit or worktree does.
func acceptanceCurrent(phase string, recorded, current Fingerprint) bool {
	if phaseIndex(phase) >= phaseIndex("pr_ready") {
		return sameContent(recorded, current)
	}
	return sameFingerprint(recorded, current)
}

// acceptanceFindings reports a rejection while the work is being redone, and drops an
// acceptance that no longer describes the current content.
func acceptanceFindings(snap *Snapshot) {
	a := snap.State.OwnerAcceptance
	if a == nil {
		return
	}
	if a.Verdict == "rejected" {
		if snap.State.Phase == "implementing" {
			snap.Findings = append(snap.Findings, Finding{"owner.rejected", "product", "failed", "The owner rejected the product: " + a.Summary})
		}
		return
	}
	if !acceptanceCurrent(snap.State.Phase, a.Fingerprint, snap.Fingerprint) {
		snap.State.OwnerAcceptance = nil
	}
}
