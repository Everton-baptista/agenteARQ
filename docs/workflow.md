# Product workflow: host plugins, one verifiable project

The coding host (Codex, Claude or Gemini) runs the conversation and implementation. AgentARQ
stores the product contract, tells the host which evidence is missing, and checks the result.
It never starts a model, launches another coding host, publishes a PR, merges or deploys.
The same Go service backs CLI, local MCP and the optional local progress panel.

## Artifacts and independent subjects

`agentarch/project/product.yaml` describes a product, not an AI agent. `has_ai: false` is a valid
application: no fictional application-agent manifest, model or prompt is required. Development
executors named in `executor_ids` are evaluated separately regardless of `has_ai`. Application
agents are named in `application_agent_ids`; both kinds keep manifests in the existing
`agentarch/project/agents/<id>/agent.yaml` locations. One ID cannot represent both subjects.
An undeclared executor is not certified: the product gate only reports the agents explicitly
listed and checked. Host capabilities that cannot be observed must not be presented as enforced.

The engineering profile is separate from the `minimal`, `standard` and `regulated` AI policy
profiles. Built-in engineering profiles are `typescript-web`, `python-api` and `go-service`. `universal` uses
explicit reviewed checks for another stack. Built-in checks cannot be removed by configuration.

Version and review these artifacts:

- `product.yaml`: name, intended outcome, author, engineering profile and AI subject references.
- `brief.md`: problem, users, success measures, constraints and exclusions.
- `spec.md`: observable acceptance criteria named `AC-1`, `AC-2`, etc.
- `adr/0001-architecture.md`: decision, alternatives, boundaries and consequences.
- `tasks.yaml`: tasks, dependencies and acceptance-to-check mapping.

A task's `checks` list declares coverage; it is not proof that its test is semantically adequate.
An independent reviewer must check that adequacy. Example:

```yaml
schema_version: '1.0'
tasks:
  - id: customer-search
    title: Implement tenant-scoped customer search
    acceptance: [AC-1, AC-2]
    checks: [typecheck, test, security]
    depends_on: []
```

Init refuses to overwrite existing artifacts. An existing complete product can be adopted into
a new worktree; that worktree starts at draft and must establish its own evidence. Git must be
initialized before the service is opened, and the root must be a Git worktree root.

## Checkpoints and truthful completion

The sequence is `draft → specified → planned → implementing → verifying → reviewing → pr_ready`.
`blocked` and `cancelled` require a reason. Reopening cancelled work starts at draft.
The host records task completion during implementation, after dependencies are completed.
Only the verifier advances to reviewing; only a separate reviewer can advance to pr_ready.
No checkpoint request can directly claim either result. A review can request changes instead.

State is private local metadata under the worktree's Git directory, in
`agentarch-workflow/state.json`; it is neither repository content nor chat history. Atomic replace
and an OS file lock protect updates, including simultaneous CLI/MCP calls. The OS releases the
lock after interruption. `expected_revision` enables optimistic concurrency; stale requests fail.
The most recent 1,000 checkpoint entries are retained. No model input/output is stored.

Evidence binds HEAD, branch, worktree path, Git directory, profile and hashes of tracked plus
untracked non-ignored source files. Deletes, symlinks and executable permissions participate.
Generated evidence exports under `agentarch/project/evidence/` are excluded from the source hash
so exporting results cannot invalidate itself. Local state remains authoritative; editing an
exported report cannot grant readiness. A source, commit, branch, profile or worktree change
invalidates verification and review; Status reports verifying until checks and review run again.
This evidence is local engineering evidence, not cryptographically authenticated certification.

Approval requires an existing HEAD and a clean source worktree (evidence exports excluded).
Commit changes before final verification and review. `RecordPR` records a host-published HTTPS PR
URL belonging to the configured origin and the exact reviewed HEAD; it never publishes anything.

The review record requires `reviewer != author`, a human/independent-agent kind, a verdict and
an evidence summary. Identity is an explicit attestation from the host/user, not proof that two
names represent independent people. The adapter must obtain a real separate reviewer; it must
not fabricate one. There is no automatic merge or production deployment.

## Isolated verification

Default verification requires Podman or Docker and a locally installed verifier image named by
an immutable `@sha256:` digest. The image must already contain the profile dependencies. There
is **no local fallback** and **no image pull**. Missing image, runtime or executable produces an
inconclusive result, which blocks readiness instead of becoming a pass.

Each command receives a disposable copy of the Git-visible source, without `.git`, ignored
secrets, host credentials or a Docker socket. Network is disabled, root filesystem is read-only,
capabilities are dropped, new privileges prohibited, and CPU, memory, processes and time bounded.
A generated source archive is streamed through stdin; no host directory is mounted. Only bounded in-container `/workspace` and `/tmp` are writable. A timeout removes the container.
The real worktree fingerprint is compared before and after the run; a concurrent edit invalidates
that run. Commands can execute project code inside this sandbox; they are never evaluated by a
host shell. A fixed in-container shell wrapper extracts that archive into bounded tmpfs and executes
positional arguments without interpolating them into shell source. The image needs `/bin/sh` and `tar`. Container isolation is a prerequisite, not a claim of perfect containment.

TypeScript web requires npm scripts `lint`, `typecheck`, `test` (non-watch `--run`), `build`,
`test:e2e`, `test:a11y`, and `test:security`. Python API requires Ruff, mypy, pytest, pip dependency
consistency and Bandit. Go service requires gofmt-clean source, `go vet`, `go build` and
`go test -race`, with modules from the image's read-only cache (`GOPROXY=off`). Browser binaries, test databases and dependencies must be provisioned in
the verifier image or tests; this first version deliberately provides no network access.

Every profile, `universal` included, also runs the **security floor**: `secrets` (gitleaks over
the staged source) and `dependency-audit` (osv-scanner against an offline OSV database baked into
the image). Their configuration belongs to the image: a `.gitleaks.toml`, `.gitleaksignore` or
`osv-scanner.toml` in the project is ignored, and product.yaml cannot remove, rename or redefine
either check. An image without the floor, a scanner error, a database older than 14 days, or
dependency manifests without an auditable lockfile all produce `inconclusive`, which blocks
readiness. A project with no dependency manifests passes the audit with nothing to check.

For another stack, the universal product declares `checks`, each with `id`, `program`, `args`
and `timeout_seconds` (1–600). Changing these checks changes the source fingerprint. This is a
reviewed project contract, not executable code inside an imported policy pack.

Checks retain status, duration, exit code, total output byte count and a SHA-256 of the
complete merged stdout/stderr stream. The default is content-free; raw stdout/stderr is never
persisted. A nonzero exit cannot pass. Any failed or inconclusive required check blocks readiness.
Executor and application-agent gates use the existing policy engine through an explicit adapter;
if that adapter is unavailable for declared agents, verification is inconclusive.

## Validation and benchmark

`go test ./internal/workflow` includes a twelve-scenario deterministic workflow acceptance suite:
ordinary product, AI executor separation, missing infrastructure, stale source, source changed
during checking, dependency ordering, phase skipping, self-review, concurrent writes, stale
revision, host restart and requested changes. Its runners are labeled `simulated`; these tests
prove state and gate behavior, not coding-model quality or measured productivity improvement.
OCI command construction and source staging are separately tested without contacting a daemon.
A live OCI run remains dependent on a real installed, pinned verifier image.

## Reproducing the live OCI smoke

`scripts/test-workflow-live.sh` uses a temporary product repository and an already installed
image passed through `IMAGE`. Set `AGENTARCH_BIN` to the built binary and optionally `KEEP_TMP=1`
to keep its JSON evidence. The script does not pull an image itself and touches no existing
containers. It checks source/credential/network isolation, the gate and independent-review
**fixture**, invalidation after an edit, timeout handling and removal of its labeled container.
This is a real infrastructure integration test; the reviewer identity is fixture data, and the
result says nothing about the quality of an AI model or an actual human review of a product.

On 2026-11-16 this passed with Docker 29.7.2 and
`alpine@sha256:469c9ea410b9215eaee7976ba6c0639039b444be8e5d638f7abbada189971eb2`.
Streaming the source archive also verified operation with a daemon that does not share the
caller's filesystem; a bind-mounted source approach would not have worked in that environment.


## Explicit diagnostics for synthetic test fixtures

`workflow verify` and `verify-profile` accept `--diagnostics-reason TEXT` when an
operator explicitly needs compiler/test output to repair synthetic public fixtures.
The default remains hash-only. An empty or invalid reason is rejected; the reason
is recorded in verification evidence.

With this option, each check can include `output_excerpt`, `output_bytes` and
`output_truncated`. Capture keeps at most the first 64 KiB in memory, removes an
incomplete boundary line when truncated, redacts recognizable credential patterns,
and returns at most 16 KiB. The SHA-256 and byte count still cover the full stream.
The excerpt is stored with the verification record and its normal evidence export;
no raw log file is created. Redaction is best effort, not a complete secret or PII
detector. Use fixtures without sensitive data; do not enable this for private user
records merely because redaction exists.

This is explicit capture of verification-process stdout/stderr, not model prompts,
responses or chat transcripts. It grants no extra network, filesystem or container
permissions and is not exposed as a new MCP authority. Both benchmark conditions
use the same opt-in policy when enabled by their trusted coordinator.
