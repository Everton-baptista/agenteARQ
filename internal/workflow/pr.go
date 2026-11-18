package workflow

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var regexpPR = regexp.MustCompile(`^(pull|pull-requests|-/merge_requests)/[1-9][0-9]*$`)

func (s *Service) sourceClean() (bool, error) {
	changed, e := git(s.Root, "diff", "HEAD", "--name-only", "-z", "--")
	if e != nil {
		return false, e
	}
	untracked, e := git(s.Root, "ls-files", "--others", "--exclude-standard", "-z")
	if e != nil {
		return false, e
	}
	for _, p := range strings.Split(changed+untracked, "\x00") {
		if p != "" && !strings.HasPrefix(p, "agentarch/project/evidence/") {
			return false, nil
		}
	}
	return true, nil
}

// RecordPR records a host-created PR; it does not contact a remote or publish anything.
func (s *Service) RecordPR(req PRRequest) (Snapshot, error) {
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
	if st.Phase != "pr_ready" || st.Review == nil || st.Review.Verdict != "approved" || !sameFingerprint(st.Review.Fingerprint, fp) || req.Head != fp.Head {
		return Snapshot{}, errors.New("PR must reference the exact verified and independently reviewed commit")
	}
	if clean, err := s.sourceClean(); err != nil || !clean {
		return Snapshot{}, errors.New("PR source worktree must be clean")
	}
	u, e := url.Parse(req.URL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Snapshot{}, errors.New("PR URL must be a plain HTTPS URL")
	}
	remote, e := git(s.Root, "remote", "get-url", "origin")
	if e != nil {
		return Snapshot{}, errors.New("configure origin before recording a PR URL")
	}
	remote = strings.TrimSpace(remote)
	var host, repo string
	if ru, err := url.Parse(remote); err == nil && ru.Host != "" {
		host = ru.Host
		repo = ru.Path
	} else if at := strings.Index(remote, "@"); at >= 0 {
		rest := remote[at+1:]
		if colon := strings.Index(rest, ":"); colon >= 0 {
			host = rest[:colon]
			repo = "/" + rest[colon+1:]
		}
	}
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if host == "" || !strings.EqualFold(host, u.Host) || !strings.HasPrefix(u.Path, repo+"/") {
		return Snapshot{}, errors.New("PR URL must belong to the repository configured as origin")
	}
	tail := strings.TrimPrefix(u.Path, repo+"/")
	if !regexpPR.MatchString(tail) {
		return Snapshot{}, errors.New("URL must identify a pull request or merge request")
	}
	st.PR = &PRRecord{URL: u.String(), Head: fp.Head, At: now()}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}
