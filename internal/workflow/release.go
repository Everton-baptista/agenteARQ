package workflow

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Production phases. AgenteARQ never deploys: a person or CI system ships, and these
// records hold the evidence that it happened, which content it shipped, and what was seen
// in production afterwards. Nothing here contacts a remote.

const ReleasePlanPath = "agentarch/project/release.md"

// Each section must exist and say something; the plan is part of the reviewed source, so a
// reviewer has read how to roll back before anything ships.
var releaseSections = []string{"Environment", "Owner", "Rollback", "Post-deploy verification", "Signals"}

// releaseAliases accepts each section in the languages the interview speaks.
var releaseAliases = map[string][]string{
	"Environment":              {"environment", "ambiente", "entorno"},
	"Owner":                    {"owner", "responsável", "responsable"},
	"Rollback":                 {"rollback", "reversão", "reversión"},
	"Post-deploy verification": {"post-deploy verification", "verificação pós-deploy", "verificación posterior al despliegue"},
	"Signals":                  {"signals", "sinais", "señales"},
}

var environmentID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

type ReleaseRequest struct {
	URL              string  `json:"url"`
	Head             string  `json:"head"`
	Environment      string  `json:"environment"`
	ExpectedRevision *uint64 `json:"expected_revision,omitempty"`
}

type ReleaseRecord struct {
	URL          string    `json:"url"`
	Head         string    `json:"head"`
	Environment  string    `json:"environment"`
	SourceSHA256 string    `json:"source_sha256"`
	At           time.Time `json:"at"`
}

type ProductionCheckRequest struct {
	URL              string  `json:"url"`
	Outcome          string  `json:"outcome"`
	Attester         string  `json:"attester"`
	Summary          string  `json:"summary"`
	ExpectedRevision *uint64 `json:"expected_revision,omitempty"`
}

// ProductionCheckRecord is attested, not executed: verification runs without network, so
// AgenteARQ cannot observe production itself and does not claim to.
type ProductionCheckRecord struct {
	URL      string    `json:"url"`
	Outcome  string    `json:"outcome"`
	Attester string    `json:"attester"`
	Summary  string    `json:"summary"`
	Head     string    `json:"head"`
	At       time.Time `json:"at"`
}

// sameContent compares what was built, ignoring which commit or branch carries it, so a
// squash or rebase merge of exactly the reviewed tree is still the reviewed tree.
func sameContent(a, b Fingerprint) bool {
	return a.SourceSHA256 == b.SourceSHA256 && a.Profile == b.Profile && a.Worktree == b.Worktree
}

func plainHTTPS(raw, what string) (string, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%s must be a plain HTTPS URL", what)
	}
	return u.String(), nil
}

// releasePlanGaps lists what the release plan is missing.
func (s *Service) releasePlanGaps() []string {
	path, e := safePath(s.Root, ReleasePlanPath)
	if e != nil {
		return []string{e.Error()}
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return []string{ReleasePlanPath + " is missing; write it before verification so it is reviewed with the code"}
	}
	sections := map[string]string{}
	current := ""
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "## ") {
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	var gaps []string
	for _, name := range releaseSections {
		body, ok := findSection(sections, releaseAliases[name])
		if !ok {
			gaps = append(gaps, fmt.Sprintf("%s needs a \"## %s\" section", ReleasePlanPath, name))
		} else if !meaningful(body) {
			gaps = append(gaps, fmt.Sprintf("%s section %q must be filled in without TODO", ReleasePlanPath, name))
		}
	}
	return gaps
}

// RecordRelease records a deployment performed elsewhere. It requires that the deployed
// commit carries exactly the verified and independently reviewed content.
func (s *Service) RecordRelease(req ReleaseRequest) (Snapshot, error) {
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
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	if st.Phase != "pr_ready" || st.Review == nil || st.Review.Verdict != "approved" || st.Verification == nil || !st.Verification.Passed {
		return Snapshot{}, errors.New("a release needs passed verification and an approved independent review (phase pr_ready)")
	}
	if !sameContent(st.Review.Fingerprint, fp) || !sameContent(st.Verification.Fingerprint, fp) {
		return Snapshot{}, errors.New("the released source differs from what was verified and reviewed; verify and review again")
	}
	if strings.TrimSpace(req.Head) == "" || req.Head != fp.Head {
		return Snapshot{}, errors.New("release --head must be the commit checked out here, the one that was deployed")
	}
	if clean, err := s.sourceClean(); err != nil || !clean {
		return Snapshot{}, errors.New("release source worktree must be clean")
	}
	if gaps := s.releasePlanGaps(); len(gaps) > 0 {
		return Snapshot{}, fmt.Errorf("release plan incomplete: %s", strings.Join(gaps, "; "))
	}
	if !environmentID.MatchString(req.Environment) {
		return Snapshot{}, errors.New("environment must be a short lowercase id such as production or staging")
	}
	u, e := plainHTTPS(req.URL, "release URL (the deployment run or record)")
	if e != nil {
		return Snapshot{}, e
	}
	st.Release = &ReleaseRecord{URL: u, Head: fp.Head, Environment: req.Environment, SourceSHA256: fp.SourceSHA256, At: now()}
	st.ProductionCheck = nil
	st.Phase = "released"
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}

// RecordProductionCheck records the post-deploy verification described in the release plan.
// A failed check keeps the work released and points at the rollback plan.
func (s *Service) RecordProductionCheck(req ProductionCheckRequest) (Snapshot, error) {
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
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	if st.Phase != "released" || st.Release == nil {
		return Snapshot{}, errors.New("record a release before its production check")
	}
	if fp.SourceSHA256 != st.Release.SourceSHA256 {
		return Snapshot{}, errors.New("the source changed after release; a production check must describe the released content")
	}
	if req.Outcome != "passed" && req.Outcome != "failed" {
		return Snapshot{}, errors.New("outcome must be passed or failed")
	}
	attester := strings.TrimSpace(req.Attester)
	if attester == "" || len(attester) > 120 {
		return Snapshot{}, errors.New("attester must name who checked production")
	}
	if !meaningful(req.Summary) || len(req.Summary) > 2000 {
		return Snapshot{}, errors.New("summary must say what was checked and what was observed, without TODO")
	}
	u, e := plainHTTPS(req.URL, "production check URL (dashboard, log or report)")
	if e != nil {
		return Snapshot{}, e
	}
	st.ProductionCheck = &ProductionCheckRecord{URL: u, Outcome: req.Outcome, Attester: attester, Summary: strings.TrimSpace(req.Summary), Head: st.Release.Head, At: now()}
	if req.Outcome == "passed" {
		st.Phase = "production_verified"
	}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}

// ReleasePlanTemplate is created with the product. Its TODO markers keep a release from being
// recorded until each section says something real.
const ReleasePlanTemplate = `# Release plan

## Environment

TODO: where it runs in production (provider, region, URL) and how it is deployed.

## Owner

TODO: the person accountable for this release and for rolling it back.

## Rollback

TODO: the exact steps to return to the previous version, and how you confirm it worked.

## Post-deploy verification

TODO: what you check right after deploying, and where (smoke test, page, dashboard).

## Signals

TODO: what you watch afterwards, and for how long (errors, latency, sign-ups, complaints).
`

const ReleasePlanTemplatePT = `# Plano de release

## Ambiente

TODO: onde roda em produção (provedor, região, URL) e como é feito o deploy.

## Responsável

TODO: a pessoa responsável por este release e por desfazê-lo.

## Reversão

TODO: os passos exatos para voltar à versão anterior, e como confirmar que funcionou.

## Verificação pós-deploy

TODO: o que você confere logo depois do deploy, e onde (teste rápido, página, painel).

## Sinais

TODO: o que você acompanha depois, e por quanto tempo (erros, lentidão, cadastros, reclamações).
`
