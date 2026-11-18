package workflow

import "encoding/json"

// Evidence exports contain no check output or chat transcript. They are reviewable
// artifacts; only the local verifier record, not a hand-edited export, grants readiness.
type Evidence struct {
	SchemaVersion string        `json:"schema_version"`
	Phase         string        `json:"phase"`
	Verification  *Verification `json:"verification"`
	Review        *ReviewRecord `json:"review,omitempty"`
	PR            *PRRecord     `json:"pr,omitempty"`
}

const EvidencePath = "agentarch/project/evidence/latest.json"

func (s *Service) exportEvidence(st State) error {
	p, e := safePath(s.Root, EvidencePath)
	if e != nil {
		return e
	}
	b, e := json.MarshalIndent(Evidence{SchemaVersion: SchemaVersion, Phase: st.Phase, Verification: st.Verification, Review: st.Review, PR: st.PR}, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(p, append(b, '\n'), 0644)
}
