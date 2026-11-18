// Package workflow coordinates reviewable product artifacts, never model execution.
// Read operations are Status/Next. Init, Checkpoint, Verify and Review have effect write;
// RecordPR, RecordRelease and RecordProductionCheck only record what a person or CI did
// elsewhere: none creates a PR, sends a message, merges, deploys or starts a coding agent.
package workflow

import (
	"context"
	"time"
)

const SchemaVersion = "1.0"
const ProductPath = "agentarch/project/product.yaml"
const TasksPath = "agentarch/project/tasks.yaml"

var Phases = []string{"draft", "specified", "planned", "implementing", "verifying", "reviewing", "pr_ready", "released", "production_verified"}

type Product struct {
	SchemaVersion       string      `json:"schema_version" yaml:"schema_version"`
	Name                string      `json:"name" yaml:"name"`
	Goal                string      `json:"goal" yaml:"goal"`
	Profile             string      `json:"profile" yaml:"profile"`
	Author              string      `json:"author" yaml:"author"`
	HasAI               bool        `json:"has_ai" yaml:"has_ai"`
	ExecutorIDs         []string    `json:"executor_ids" yaml:"executor_ids"`
	ApplicationAgentIDs []string    `json:"application_agent_ids" yaml:"application_agent_ids"`
	Checks              []CheckSpec `json:"checks,omitempty" yaml:"checks,omitempty"`
}

type Task struct {
	ID         string   `json:"id" yaml:"id"`
	Title      string   `json:"title" yaml:"title"`
	Acceptance []string `json:"acceptance" yaml:"acceptance"`
	DependsOn  []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Checks     []string `json:"checks" yaml:"checks"`
}

type TaskDocument struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	Tasks         []Task `json:"tasks" yaml:"tasks"`
}

type Fingerprint struct {
	Worktree     string `json:"worktree"`
	GitDir       string `json:"git_dir"`
	Head         string `json:"head"`
	Branch       string `json:"branch"`
	SourceSHA256 string `json:"source_sha256"`
	Profile      string `json:"profile"`
}

type Finding struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Status  string `json:"status"` // failed | inconclusive | stale
	Message string `json:"message"`
}

type CheckResult struct {
	AgentID         string `json:"agent_id,omitempty"`
	ID              string `json:"id"`
	Subject         string `json:"subject"`
	Status          string `json:"status"` // passed | failed | inconclusive
	Message         string `json:"message,omitempty"`
	ExitCode        int    `json:"exit_code"`
	DurationMS      int64  `json:"duration_ms"`
	OutputSHA256    string `json:"output_sha256,omitempty"`
	OutputBytes     int64  `json:"output_bytes"`
	OutputExcerpt   string `json:"output_excerpt,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"` // Excerpt only; the digest covers the complete stream.
}

type Verification struct {
	Fingerprint       Fingerprint   `json:"fingerprint"`
	StartedAt         time.Time     `json:"started_at"`
	FinishedAt        time.Time     `json:"finished_at"`
	Isolation         string        `json:"isolation"`
	Checks            []CheckResult `json:"checks"`
	Passed            bool          `json:"passed"`
	DiagnosticsReason string        `json:"diagnostics_reason,omitempty"`
}

type ReviewRecord struct {
	Reviewer    string      `json:"reviewer"`
	Kind        string      `json:"kind"` // human | independent_agent
	Verdict     string      `json:"verdict"`
	Summary     string      `json:"summary"`
	Fingerprint Fingerprint `json:"fingerprint"`
	At          time.Time   `json:"at"`
}

type CheckpointRecord struct {
	Phase  string    `json:"phase"`
	TaskID string    `json:"task_id,omitempty"`
	Note   string    `json:"note,omitempty"`
	At     time.Time `json:"at"`
}

type State struct {
	SchemaVersion  string             `json:"schema_version"`
	Revision       uint64             `json:"revision"`
	Phase          string             `json:"phase"`
	Worktree       string             `json:"worktree"`
	CompletedTasks []string           `json:"completed_tasks"`
	Checkpoints    []CheckpointRecord `json:"checkpoints"`
	Verification   *Verification      `json:"verification,omitempty"`
	Review         *ReviewRecord      `json:"review,omitempty"`
	PR             *PRRecord          `json:"pr,omitempty"`
	// Release and ProductionCheck record shipping done elsewhere; AgenteARQ never deploys.
	Release         *ReleaseRecord         `json:"release,omitempty"`
	ProductionCheck *ProductionCheckRecord `json:"production_check,omitempty"`
}

type NextAction struct {
	Phase          string   `json:"phase"`
	Action         string   `json:"action"`
	Missing        []string `json:"missing"`
	TaskID         string   `json:"task_id,omitempty"`
	ReadyTaskIDs   []string `json:"ready_task_ids,omitempty"`
	BlockedTaskIDs []string `json:"blocked_task_ids,omitempty"` // Incomplete tasks with unfinished dependencies.
}

type Snapshot struct {
	Product     Product     `json:"product"`
	Tasks       []Task      `json:"tasks"`
	State       State       `json:"state"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Findings    []Finding   `json:"findings"`
	Next        NextAction  `json:"next"`
}

type InitOptions struct {
	Name                string   `json:"name"`
	Profile             string   `json:"profile"`
	Goal                string   `json:"goal"`
	Author              string   `json:"author"`
	HasAI               bool     `json:"has_ai"`
	ExecutorIDs         []string `json:"executor_ids"`
	ApplicationAgentIDs []string `json:"application_agent_ids"`
}

type CheckpointRequest struct {
	Phase            string  `json:"phase"`
	TaskID           string  `json:"task_id,omitempty"`
	Note             string  `json:"note,omitempty"`
	ExpectedRevision *uint64 `json:"expected_revision,omitempty"`
}

type CheckSpec struct {
	ID             string   `json:"id" yaml:"id"`
	Program        string   `json:"program" yaml:"program"`
	Args           []string `json:"args" yaml:"args"`
	TimeoutSeconds int      `json:"timeout_seconds" yaml:"timeout_seconds"`
}

type Command struct {
	ID      string        `json:"id"`
	Program string        `json:"program"`
	Args    []string      `json:"args"`
	Timeout time.Duration `json:"-"`
}

// Runner runs a fixed profile check in an isolated source snapshot. Custom runners are
// an embedding/test seam, never accepted through JSON, MCP or browser request data.
type Runner interface {
	Run(context.Context, string, Command) CheckResult
	Isolation() string
}

type AgentChecker func(context.Context, string, []string) ([]CheckResult, error)

type VerifyOptions struct {
	Runner            Runner       `json:"-"`
	AgentChecker      AgentChecker `json:"-"`
	Image             string       `json:"image,omitempty"`              // OCI image pinned with @sha256:...
	DiagnosticsReason string       `json:"diagnostics_reason,omitempty"` // Capture test output only with an explicit reason.
	ExpectedRevision  *uint64      `json:"expected_revision,omitempty"`
}

type ReviewRequest struct {
	Reviewer         string  `json:"reviewer"`
	Kind             string  `json:"kind"`
	Verdict          string  `json:"verdict"`
	Summary          string  `json:"summary"`
	ExpectedRevision *uint64 `json:"expected_revision,omitempty"`
}

type PRRequest struct {
	URL              string  `json:"url"`
	Head             string  `json:"head"`
	ExpectedRevision *uint64 `json:"expected_revision,omitempty"`
}
type PRRecord struct {
	URL  string    `json:"url"`
	Head string    `json:"head"`
	At   time.Time `json:"at"`
}
