package workflow

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var slug = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var criterion = regexp.MustCompile(`^AC-[0-9]+$`)
var documents = []string{"agentarch/project/brief.md", "agentarch/project/spec.md", "agentarch/project/adr/0001-architecture.md"}

func (s *Service) Init(o InitOptions) (Snapshot, error) {
	unlock, e := s.lock()
	if e != nil {
		return Snapshot{}, e
	}
	defer unlock()
	if _, e = os.Stat(s.statePath()); e == nil {
		return Snapshot{}, errors.New("workflow already initialized; use status or checkpoint")
	}
	var p Product
	// Do not overwrite an adopted product or partially initialized artifacts. A clone or a
	// new worktree adopts the committed product.yaml, so its facts need not be typed again;
	// a value that contradicts it is an error rather than silently ignored.
	if _, e = os.Stat(s.Root + "/" + ProductPath); e == nil {
		existing, _, err := s.loadArtifacts()
		if err != nil {
			return Snapshot{}, err
		}
		if name := strings.TrimSpace(o.Name); name != "" && name != existing.Name {
			return Snapshot{}, fmt.Errorf("product.yaml already names this product %q; omit --name to adopt it", existing.Name)
		}
		if o.Profile != "" && o.Profile != existing.Profile {
			return Snapshot{}, fmt.Errorf("product.yaml already uses profile %q; omit --profile to adopt it", existing.Profile)
		}
		p = existing
	} else if !os.IsNotExist(e) {
		return Snapshot{}, e
	} else {
		if strings.TrimSpace(o.Name) == "" {
			return Snapshot{}, errors.New("product name is required")
		}
		if o.Profile == "" {
			return Snapshot{}, fmt.Errorf("choose an engineering profile with --profile; none is chosen for you:\n%s", ProfileChoiceText())
		}
		if _, ok := profiles[o.Profile]; !ok {
			return Snapshot{}, fmt.Errorf("unknown engineering profile %q", o.Profile)
		}
		p = Product{SchemaVersion: SchemaVersion, Name: o.Name, Goal: o.Goal, Profile: o.Profile, Author: o.Author, HasAI: o.HasAI, ExecutorIDs: o.ExecutorIDs, ApplicationAgentIDs: o.ApplicationAgentIDs}
		pb, err := yaml.Marshal(p)
		if err != nil {
			return Snapshot{}, err
		}
		if err = validateArtifact(pb, "product.schema.json"); err != nil {
			return Snapshot{}, err
		}
		tb, _ := yaml.Marshal(TaskDocument{SchemaVersion: SchemaVersion, Tasks: []Task{}})
		files := map[string][]byte{
			ProductPath: pb, TasksPath: tb,
			documents[0]: []byte(templateFor(o.Language, BriefTemplate, BriefTemplatePT)),
			documents[1]: []byte(templateFor(o.Language, SpecTemplate, SpecTemplatePT)),
			documents[2]: []byte(templateFor(o.Language, ADRTemplate, ADRTemplatePT)),
		}
		// The release plan is reviewed with the code, so it has to exist before verification.
		// Created up front, it is not discovered at release time, after the review it needed.
		if rp, err := safePath(s.Root, ReleasePlanPath); err == nil {
			if _, err = os.Stat(rp); os.IsNotExist(err) {
				files[ReleasePlanPath] = []byte(templateFor(o.Language, ReleasePlanTemplate, ReleasePlanTemplatePT))
			}
		}
		for rel := range files {
			path, err := safePath(s.Root, rel)
			if err != nil {
				return Snapshot{}, err
			}
			if _, err = os.Stat(path); err == nil {
				return Snapshot{}, fmt.Errorf("refusing to overwrite %s", rel)
			} else if !os.IsNotExist(err) {
				return Snapshot{}, err
			}
		}
		for rel, b := range files {
			path, _ := safePath(s.Root, rel)
			if err := atomicWrite(path, b, 0644); err != nil {
				return Snapshot{}, err
			}
		}
	}
	st := State{SchemaVersion: SchemaVersion, Phase: "draft", Worktree: s.Root, CompletedTasks: []string{}, Checkpoints: []CheckpointRecord{}}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}

func (s *Service) Status() (Snapshot, error) { return s.status() }
func (s *Service) Next() (NextAction, error) { snap, e := s.status(); return snap.Next, e }
func (s *Service) status() (Snapshot, error) {
	p, t, e := s.loadArtifacts()
	if e != nil {
		return Snapshot{}, e
	}
	st, e := s.loadState()
	if e != nil {
		return Snapshot{}, e
	}
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	snap := Snapshot{Product: p, Tasks: t.Tasks, State: st, Fingerprint: fp, Findings: []Finding{}}
	// From pr_ready on, the reviewed content may arrive in a different commit (a squash or
	// rebase merge), so only a content change makes the evidence stale. Before that, any
	// change of source, HEAD, branch, worktree or profile does.
	if st.Verification != nil {
		stale := !sameFingerprint(st.Verification.Fingerprint, fp)
		if phaseIndex(st.Phase) >= phaseIndex("pr_ready") {
			stale = !sameContent(st.Verification.Fingerprint, fp)
		}
		if stale {
			snap.Findings = append(snap.Findings, Finding{"evidence.stale", "product", "stale", "Source, HEAD, worktree or profile changed; verify and review again."})
			if phaseIndex(st.Phase) >= phaseIndex("reviewing") {
				snap.State.Phase = "verifying"
			}
		}
	}
	if st.ProductionCheck != nil && st.ProductionCheck.Outcome == "failed" && snap.State.Phase == "released" {
		snap.Findings = append(snap.Findings, Finding{"production.check_failed", "product", "failed", "The production check failed; follow the rollback in " + ReleasePlanPath + "."})
	}
	acceptanceFindings(&snap)
	if st.Verification != nil {
		for _, check := range st.Verification.Checks {
			if check.Status != "passed" {
				snap.Findings = append(snap.Findings, Finding{check.ID, check.Subject, check.Status, check.Message})
			}
		}
	}
	snap.Next = s.nextFor(snap)
	return snap, nil
}

func (s *Service) nextFor(snap Snapshot) NextAction {
	phase := snap.State.Phase
	missing := []string{}
	action := ""
	next := NextAction{Phase: phase}
	switch phase {
	case "draft":
		missing = s.requirements(snap.Product, snap.Tasks, "specified")
		action = "Complete the brief and specification; checkpoint specified."
		// One thing at a time: while the brief and spec are being written, only the advice
		// that belongs to the spec. The rest arrives at the step where it is acted on.
		if !s.exists(ScenariosPath) {
			next.Advice = []string{AdviceScenarios}
		}
	case "specified":
		missing = s.requirements(snap.Product, snap.Tasks, "planned")
		action = "Record the architecture decision and acceptance-to-check task mapping; checkpoint planned."
		next.Advice = s.planningAdvice(snap.Product)
	case "planned":
		action = "Use a dedicated worktree and begin implementation; checkpoint implementing."
	case "implementing":
		for _, t := range snap.Tasks {
			if contains(snap.State.CompletedTasks, t.ID) {
				continue
			}
			var pending []string
			for _, dependency := range t.DependsOn {
				if !contains(snap.State.CompletedTasks, dependency) {
					pending = append(pending, dependency)
				}
			}
			if len(pending) > 0 {
				next.BlockedTaskIDs = append(next.BlockedTaskIDs, t.ID)
				missing = append(missing, "task "+t.ID+" depends on incomplete "+strings.Join(pending, ", "))
				continue
			}
			next.ReadyTaskIDs = append(next.ReadyTaskIDs, t.ID)
			missing = append(missing, "task "+t.ID)
		}
		if len(next.ReadyTaskIDs) > 0 {
			next.TaskID = next.ReadyTaskIDs[0]
			action = "Implement task " + next.TaskID + " and checkpoint it when complete."
			for _, t := range snap.Tasks {
				if t.ID == next.TaskID {
					next.Context = s.taskContext(t)
				}
			}
		} else if len(next.BlockedTaskIDs) > 0 {
			action = "Resolve the unfinished task dependencies before continuing implementation."
		} else {
			action = "All planned tasks are complete; run verification."
		}
		next.Advice = append(append(releasePlanReminder(s.releasePlanGaps()), s.appRiskAdvice()...), s.tokenColourAdvice()...)
	case "verifying":
		action = "Run the engineering profile in an isolated container; fix failed or inconclusive checks."
		next.Advice = append(append(releasePlanReminder(s.releasePlanGaps()), s.appRiskAdvice()...), s.tokenColourAdvice()...)
	case "reviewing":
		action = "Request an independent human or agent review of this exact fingerprint."
		next.Advice = append(s.appRiskAdvice(), s.tokenColourAdvice()...)
	case "pr_ready":
		action = "The local gate passed. Publish the PR with your host; after a person or CI deploys this content, record it with record-release. This tool does not merge or deploy."
		if gaps := s.releasePlanGaps(); len(gaps) > 0 {
			action += " Releasing also needs " + ReleasePlanPath + " in the reviewed source."
		}
		if a := snap.State.OwnerAcceptance; a == nil || a.Verdict != "accepted" {
			next.Advice = append(next.Advice, AdviceAccept)
		}
		next.Advice = append(next.Advice, s.releaseRecommendations()...)
		next.Advice = append(next.Advice, s.appRiskAdvice()...)
	case "released":
		action = "Run the post-deploy verification from " + ReleasePlanPath + " and record it with record-production-check."
		next.Advice = s.releaseRecommendations()
	case "production_verified":
		action = "Shipped and checked in production. Keep watching the signals in " + ReleasePlanPath + "; new work starts at implementing."
	case "blocked":
		action = "Resolve the recorded blocker and checkpoint implementing."
	case "cancelled":
		action = "Work cancelled; explicitly checkpoint draft to reopen."
	}
	for _, f := range snap.Findings {
		missing = append(missing, f.Message)
	}
	next.Action, next.Missing = action, missing
	return next
}

func validateTasks(tasks []Task) error {
	ids := map[string]Task{}
	for _, t := range tasks {
		if !slug.MatchString(t.ID) {
			return fmt.Errorf("invalid task id %q", t.ID)
		}
		if _, ok := ids[t.ID]; ok {
			return fmt.Errorf("duplicate task id %s", t.ID)
		}
		ids[t.ID] = t
	}
	visiting := map[string]bool{}
	done := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("task dependency cycle at %s", id)
		}
		if done[id] {
			return nil
		}
		t, ok := ids[id]
		if !ok {
			return fmt.Errorf("unknown task dependency %s", id)
		}
		visiting[id] = true
		for _, d := range t.DependsOn {
			if e := visit(d); e != nil {
				return e
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	for id := range ids {
		if e := visit(id); e != nil {
			return e
		}
	}
	return nil
}
func meaningful(v string) bool {
	v = strings.TrimSpace(v)
	return len(v) >= 12 && !strings.Contains(strings.ToUpper(v), "TODO")
}
func (s *Service) requirements(p Product, tasks []Task, phase string) []string {
	missing := []string{}
	if !meaningful(p.Goal) {
		missing = append(missing, "product.goal must describe the intended outcome without TODO")
	}
	if strings.TrimSpace(p.Author) == "" {
		missing = append(missing, "product.author must identify the implementer")
	}
	for _, id := range append(append([]string{}, p.ExecutorIDs...), p.ApplicationAgentIDs...) {
		if !slug.MatchString(id) {
			missing = append(missing, "invalid agent id "+id)
		}
	}
	for _, id := range p.ExecutorIDs {
		if contains(p.ApplicationAgentIDs, id) {
			missing = append(missing, "agent cannot be both an executor and application component: "+id)
		}
	}
	if p.HasAI && len(p.ApplicationAgentIDs) == 0 {
		missing = append(missing, "declare application_agent_ids for the AI product")
	}
	if !p.HasAI && len(p.ApplicationAgentIDs) > 0 {
		missing = append(missing, "application agents contradict has_ai: false")
	}
	for _, rel := range documents[:2] {
		path, e := safePath(s.Root, rel)
		if e != nil {
			missing = append(missing, e.Error())
			continue
		}
		b, e := os.ReadFile(path)
		if e != nil || !meaningful(string(b)) {
			missing = append(missing, "complete "+rel)
		}
	}
	if phaseIndex(phase) >= phaseIndex("planned") {
		missing = append(missing, s.architectureGaps()...)
		commands, e := commandsFor(p)
		if e != nil {
			missing = append(missing, e.Error())
		}
		known := map[string]bool{}
		for _, c := range commands {
			known[c.ID] = true
		}
		specPath, _ := safePath(s.Root, documents[1])
		spec, _ := os.ReadFile(specPath)
		if len(tasks) == 0 {
			missing = append(missing, "define at least one task with acceptance criteria and checks")
		}
		for _, t := range tasks {
			if !meaningful(t.Title) {
				missing = append(missing, "task "+t.ID+" needs a concrete title")
			}
			if len(t.Acceptance) == 0 {
				missing = append(missing, "task "+t.ID+" needs acceptance criteria")
			}
			for _, a := range t.Acceptance {
				if !criterion.MatchString(a) || !regexp.MustCompile(`\b`+regexp.QuoteMeta(a)+`\b`).Match(spec) {
					missing = append(missing, "task "+t.ID+" references missing acceptance criterion "+a)
				}
			}
			if len(t.Checks) == 0 {
				missing = append(missing, "task "+t.ID+" must map acceptance criteria to verification checks")
			}
			for _, c := range t.Checks {
				if !known[c] {
					missing = append(missing, "task "+t.ID+" references unknown check "+c)
				}
			}
		}
		missing = append(missing, s.scenarioGaps(spec)...)
		missing = append(missing, s.personaGaps()...)
		missing = append(missing, s.designGaps()...)
	}
	return missing
}

func (s *Service) Checkpoint(req CheckpointRequest) (Snapshot, error) {
	if len(req.Note) > 4000 {
		return Snapshot{}, errors.New("checkpoint note exceeds 4000 bytes")
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
	if e = revisionOK(st, req.ExpectedRevision); e != nil {
		return Snapshot{}, e
	}
	if req.Phase == "" {
		req.Phase = st.Phase
	}
	if req.Phase == "reviewing" || req.Phase == "pr_ready" {
		return Snapshot{}, errors.New("only successful verification and independent review can advance to reviewing/pr_ready")
	}
	if req.Phase == "released" || req.Phase == "production_verified" {
		return Snapshot{}, errors.New("only record-release and record-production-check can advance to released/production_verified")
	}
	if req.Phase != "blocked" && req.Phase != "cancelled" && phaseIndex(req.Phase) < 0 {
		return Snapshot{}, errors.New("unknown workflow phase")
	}
	if req.Phase == "blocked" || req.Phase == "cancelled" {
		if strings.TrimSpace(req.Note) == "" {
			return Snapshot{}, errors.New("blocked/cancelled requires a reason")
		}
	} else {
		from := phaseIndex(st.Phase)
		to := phaseIndex(req.Phase)
		if st.Phase == "cancelled" && req.Phase != "draft" {
			return Snapshot{}, errors.New("cancelled work must reopen at draft")
		}
		if from >= 0 && to > from+1 {
			return Snapshot{}, errors.New("cannot skip workflow phases")
		}
		if missing := s.requirements(p, t.Tasks, req.Phase); len(missing) > 0 && req.Phase != "draft" {
			return Snapshot{}, fmt.Errorf("cannot advance: %s", strings.Join(missing, "; "))
		}
	}
	if req.TaskID != "" {
		if req.Phase != "implementing" {
			return Snapshot{}, errors.New("task completion is recorded during implementing")
		}
		var target *Task
		for i := range t.Tasks {
			if t.Tasks[i].ID == req.TaskID {
				target = &t.Tasks[i]
			}
		}
		if target == nil {
			return Snapshot{}, errors.New("unknown task id")
		}
		for _, d := range target.DependsOn {
			if !contains(st.CompletedTasks, d) {
				return Snapshot{}, fmt.Errorf("task %s depends on incomplete %s", target.ID, d)
			}
		}
		if !contains(st.CompletedTasks, req.TaskID) {
			st.CompletedTasks = append(st.CompletedTasks, req.TaskID)
		}
	}
	if req.Phase == "verifying" {
		for _, task := range t.Tasks {
			if !contains(st.CompletedTasks, task.ID) {
				return Snapshot{}, fmt.Errorf("task %s is incomplete", task.ID)
			}
		}
	}
	if req.Phase != st.Phase || req.TaskID != "" {
		st.Review = nil
		st.Release, st.ProductionCheck = nil, nil
	}
	if req.Phase != st.Phase {
		st.OwnerAcceptance = nil // a rejection stays visible while the work is redone, then clears
	}
	st.Phase = req.Phase
	st.Checkpoints = append(st.Checkpoints, CheckpointRecord{Phase: req.Phase, TaskID: req.TaskID, Note: req.Note, At: now()})
	if len(st.Checkpoints) > 1000 {
		st.Checkpoints = st.Checkpoints[len(st.Checkpoints)-1000:]
	}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}

func (s *Service) Review(req ReviewRequest) (Snapshot, error) {
	if len(req.Summary) > 8000 || len(req.Reviewer) > 200 {
		return Snapshot{}, errors.New("review exceeds bounded summary or identity size")
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
	if st.Phase != "reviewing" {
		return Snapshot{}, errors.New("successful verification is required before review")
	}
	if req.Kind != "human" && req.Kind != "independent_agent" {
		return Snapshot{}, errors.New("review kind must be human or independent_agent")
	}
	if strings.TrimSpace(req.Reviewer) == "" || strings.EqualFold(strings.TrimSpace(req.Reviewer), strings.TrimSpace(p.Author)) {
		return Snapshot{}, errors.New("reviewer must be identified and independent of the implementer")
	}
	if req.Verdict != "approved" && req.Verdict != "changes_requested" {
		return Snapshot{}, errors.New("review verdict must be approved or changes_requested")
	}
	if !meaningful(req.Summary) {
		return Snapshot{}, errors.New("review summary must describe the evidence and findings")
	}
	fp, e := s.fingerprint(p.Profile)
	if e != nil {
		return Snapshot{}, e
	}
	if st.Verification == nil || !st.Verification.Passed || !sameFingerprint(st.Verification.Fingerprint, fp) {
		return Snapshot{}, errors.New("verification evidence is stale or failed; verify again")
	}
	if req.Verdict == "approved" {
		if fp.Head == "unborn" {
			return Snapshot{}, errors.New("commit the product before approving its review")
		}
		if clean, err := s.sourceClean(); err != nil || !clean {
			return Snapshot{}, errors.New("commit source changes, then verify and review the exact clean commit")
		}
	}
	st.Review = &ReviewRecord{Reviewer: req.Reviewer, Kind: req.Kind, Verdict: req.Verdict, Summary: req.Summary, Fingerprint: fp, At: now()}
	if req.Verdict == "approved" {
		st.Phase = "pr_ready"
	} else {
		st.Phase = "implementing"
	}
	if e = s.save(st); e != nil {
		return Snapshot{}, e
	}
	return s.status()
}
func phaseIndex(p string) int {
	for i, v := range Phases {
		if p == v {
			return i
		}
	}
	return -1
}
func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

// releasePlanReminder turns release-plan gaps into advice while it can still be reviewed with the
// code: finishing it after review would make the review stale.
func releasePlanReminder(gaps []string) []string {
	out := make([]string, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, "before verifying, so it is reviewed with the code: "+g)
	}
	return out
}
