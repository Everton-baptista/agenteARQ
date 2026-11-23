# Mission and roadmap to 1.0

Status: in progress, 2026-11-23. Owner: Everton Baptista. This document states what AgenteARQ is
for, where it stands today against that purpose, and the plan to close the gap. Every exit
criterion below is meant to be checked from an artifact, not asserted.

## Mission

**Anyone with an idea, technical or not, can build the software product they want with the AI
coding assistant they already use, and reach production with the same standard of excellence
a senior engineering team would demand: specified, compared, tested, secured, independently
reviewed and released with evidence.**

AgenteARQ is the guide and the referee, never the player:

- **The guide.** At every moment the owner knows where the product stands, what happens next,
  why, and the one decision that is theirs to make, in their own language and without jargon.
  The assistant cannot get lost because the next step comes from recorded state, not from the
  conversation's memory.
- **The referee.** Nothing counts as done without evidence that a sandboxed check produced.
  The author never approves its own work. Any change after approval sends the work back.
- **Agnostic by construction.** Any product, any language, any host, any vendor. Technology is
  compared and chosen by the owner, recorded in an ADR, and verified by checks that fit the
  chosen stack.
- **Cheap and fast.** The assistant receives only the context the current step needs; state,
  summaries and refusals are computed deterministically by the CLI instead of re-read by a model.

### Positioning

AgenteARQ is **the evidence and gate layer under any spec framework and any stack**, not one
more template library. Spec Kit, OpenSpec, BMAD, Kiro and the app builders keep the truth
inside the model: the prompt says what to do and the same model decides whether it was done.
AgenteARQ moves that decision out of the model in three places: state in the CLI, evidence in
the signed sandbox, and the block in the host hook. Every public incident in the category
(open database rules, a production database deleted by an agent, a hallucinated package, a
compromised scanner) should become a check that fails, with one plain sentence telling the
owner what to do next.

### What AgenteARQ will not become

- A model, a coding host or a hosting platform. It never calls a model, merges or deploys.
- A vendor catalogue with rankings or affiliate links. Options are compared against the owner's
  constraints, with dated sources.
- A certification. Evidence is engineering evidence, and its limits are always stated.

## Assessment: is it ready today?

**No.** The core is unusually strong; the experience for a person without technical knowledge,
the breadth of agnostic verification, behavioural testing and public proof are not there yet.
"Best on the market" cannot be claimed until the benchmark below says so.

### What is already solid

- A gated workflow from `draft` to `production_verified` with evidence bound to the exact source
  hash, invalidated by any later change (`internal/workflow`).
- Isolated verification in a pinned OCI container with no network, no secrets and no `.git`, plus
  a security floor (gitleaks, osv-scanner) the project cannot disable.
- No self-approval: reviewer must differ from author and is tied to the reviewed content.
- Technology as a compared owner decision, enforced by ADR structure checks before `planned`.
- Hosts: Claude Code, Codex and Gemini CLI; skills plus a five-tool local MCP server
  (`inspect_project`, `get_workflow`, `get_next_task`, `record_checkpoint`, `verify_project`).
- A standard for AI agents (16 standards, 46 controls, 10 packs, 12 adapters) and two languages.

### Gaps, by what they cost a non-technical owner

| # | Gap | Evidence | Consequence |
|---|---|---|---|
| G1 | Prerequisites are technical | Install needs curl/Go/Docker; verification needs Docker or Podman and a prepared verifier image | Most lay users stop before the first check runs |
| G2 | Guidance lives only in prompts | Plugin declares `global_tool_enforcement: false`; no host hooks are shipped | The assistant can drift, skip steps or edit generated files; nothing stops it in real time |
| G3 | Agnosticism ends at five profiles | `typescript-web`, `python-api`, `go-service`, `static-site`, `docs`; everything else is `universal`, where the owner must name the commands | A lay user cannot write those commands; mobile, Java, .NET, PHP, Rust and data stacks get no guided checks |
| G4 | No behavioural or persona testing | Acceptance criteria map to check ids; nothing turns them into executable scenarios or exercises the product as a user would | "Tests pass" can still mean "the product does not do what the owner meant" |
| G5 | Independent review assumes a second person | Review requires `reviewer != author`; a solo owner cannot review code | Solo owners get stuck at `reviewing` or fabricate a reviewer |
| G6 | Owner acceptance is missing | No step where the owner tries the product and accepts it in plain language | The owner's own judgement is never recorded as evidence |
| G7 | Refusals speak engineer | Gate refusals name fields and phases; `agentarch explain` covers controls, not workflow refusals | A blocked lay user does not know what to do next |
| G8 | Production is left to the owner | `record-release` and `record-production-check` record, but no guided deploy, backup, monitoring or maintenance playbooks exist | The last mile, where non-technical owners need the most help, has the least |
| G9 | Token cost is unmeasured | Skills are small, but no per-phase context budget or measurement exists | Speed and cost claims cannot be made or defended |
| G10 | Proof is incomplete | Benchmark cohort at 10 of 72 runs; Claude Code and Gemini runs had no authenticated inference | "Best on the market" is a hypothesis, not a result |
| G11 | Release hygiene | `spec/1.0` is a draft and the newest content (1.4.0) is unreleased; a locally built `./agentarch` (gitignored) can lag the tree | Users can run a binary that contradicts the README |
| G12 | No design standard | Nothing guides or checks the look of a generated interface | Products look like every other AI-generated app, which costs the owner credibility |

## Principles for every change in this plan

1. **One decision at a time.** The owner is never asked more than one question per turn, always
   with options, their trade-offs and a labelled recommendation.
2. **State over memory.** Where the product stands is computed by `agentarch`, never recalled by
   the model. Resuming after a day, a crash or a new chat yields the same next step.
3. **Enforce where the host allows; say so where it does not.** Hooks and MCP enforce; prompts
   guide. Documentation never presents guidance as enforcement.
4. **Deterministic gates block; LLM judges advise.** Persona agents and LLM reviewers produce
   findings, not passes (invariant 12).
5. **Add capability as data.** New stacks, scenarios and playbooks ship as reviewed content
   recipes, not new code paths, following ADR 0001.
6. **Measure before claiming.** Every speed, cost or quality claim cites a benchmark run.

## Roadmap

Phases are ordered by what unblocks a lay user first. Each has an exit criterion that a test,
a benchmark result or a recorded pilot can check.

### Phase 0 — Truth and baseline

- Rebuild and re-sign the binary from the current tree; remove or regenerate the stale committed
  `./agentarch`; CI fails when the committed binary's content version differs from
  `content/VERSION`.
- Finish the `products-web-v2` cohort and add Claude Code and Gemini CLI with authenticated runs.
- Define the lay-user pilot: five people without programming background, each building one small
  product of their choice with Claude Code, observed against a written protocol.

Exit: CI green on a clean machine; 72/72 cohort runs recorded; pilot protocol committed with the
first two sessions' raw observations.

### Phase 1 — Never lost: guided start and resume (G1, G2, G7)

- **`agentarch doctor`**: detects every prerequisite (Git, container runtime, verifier image,
  host plugin, MCP approval) and, for each missing one, prints the plain-language reason and the
  exact next action for the user's OS. Offered automatically by `start`.
- **Plugin distributed as a Claude Code plugin** (skills, MCP server and hooks in one install),
  and the equivalent packaging for Codex and Gemini where the host supports it.
- **Host hooks where available**, all calling one command, `agentarch gate check --event <e>`,
  through a thin adapter per host (Claude Code first, shipped in the plugin's
  `hooks/hooks.json`, because a plugin's subagent definitions ignore `hooks`):
  - session start, and again after compaction: inject a deterministic, compact status (phase,
    next step, pending decision), well under the host's injected-context cap;
  - before a tool call: block edits to `agentarch:generated` files, writes of secret-shaped
    values, out-of-phase writes, and `git push`, merge or deploy commands without the owner's
    approval;
  - stop and task completion: refuse "done" while `agentarch validate` fails or no checkpoint
    was recorded for work performed.
- **A published table of each host's real boundary**: Codex runs project and plugin hooks only
  after the user trusts them; Gemini CLI lets the action through when a hook prints invalid
  output. `agentarch check` in CI remains the guarantee wherever hooks cannot be.
- **A fast path** for small changes (a typo, a copy edit) so ceremony stays proportional; the
  research's sharpest risk is BMAD-style overhead.
- **Import and export** of Spec Kit, OpenSpec and Kiro spec layouts, so AgenteARQ can gate
  work that started in another framework.
- **Three owner commands** in every host: `status` (where are we), `continue` (do the next step),
  `help` (what does this mean). Each answers in the owner's language and fits on one screen.
- **Plain-language refusal catalogue**: every workflow refusal has an id, an explanation for a
  non-technical reader and the next action; `agentarch explain <refusal>` prints it.

Exit: in the pilot, every participant reaches `planned` without outside help; a scripted test
replays a resumed session in a fresh chat and gets the same next step; every refusal id in
`internal/workflow` has a catalogue entry (test enforced).

### Phase 2 — Agnostic verification at scale (G3, G1)

- **Profiles as data**: move profile definitions into content recipes (checks, verifier recipe,
  required categories), so a new stack is a reviewed recipe, not a code change.
- **Recipe catalogue**, in order of demand: Java (Maven/Gradle), .NET, PHP, Ruby, Rust,
  Flutter/Dart, React Native, Kotlin/Android, data pipelines (SQL/dbt), infrastructure as code.
- **Guided `universal`**: when no recipe fits, the assistant proposes checks for the required
  categories (format, lint, type or static analysis, unit test, build, plus the fixed security
  floor); the owner approves them in an ADR. A `universal` product missing a required category
  is `inconclusive`, never a pass.
- **A path without local Docker**, chosen by the owner in an ADR (see open decisions): for
  example, running the same pinned image in the owner's CI with the network disabled inside the
  job, and importing its signed result. The ADR states the caveats: GitHub artifact attestations
  are free only for public repositories, and keyless cosign writes the repository identity to the
  public Sigstore log.
- **Any OCI runtime**: `docker`, `podman` (rootless, WSL2 on Windows) or Apple `container`,
  detected by `doctor`, never assumed.
- **A wider security floor**, still offline and pinned by digest:
  - Opengrep (LGPL-2.1) with an AgenteARQ-owned rule set, since Semgrep's registry rules carry a
    restrictive licence; CodeQL is not a mandatory check because private repositories need a
    paid licence;
  - mandatory lockfiles, OpenSSF malicious-package (`MAL-*`) records in the offline OSV database,
    a check that every dependency exists and is not suspiciously new (against hallucinated
    packages), and an SBOM per verification;
  - checks for the incidents that hit AI-built apps: tables without row-level security, service
    keys in the client bundle, unauthenticated routes that change data, and secret scanning of
    the build output as well as the source;
  - Checkov or KICS only when infrastructure-as-code is present, Grype or Trivy only when an
    image is in scope;
  - every scanner and CI action pinned by digest or commit SHA and its signature verified, after
    the March 2026 compromise of Trivy's release and action tags; each vulnerability database
    records its snapshot date.

Exit: each recipe has a fixture product that passes and a broken twin that fails in CI; a
pilot participant on a non-built-in stack reaches `reviewing`.

### Phase 3 — Scenarios and agentic users (G4, G6)

- **Executable acceptance**: every `AC-n` in `spec.md` maps to at least one scenario in
  `scenarios.yaml` (given, when, then), including failure, permission and edge cases. The gate
  refuses `planned` when an acceptance criterion has no scenario.
- **Personas from the brief**: `personas.yaml` describes the intended users (goal, familiarity,
  device, accessibility needs) plus fixed adversarial personas (invalid input, abuse, prompt
  injection for AI products, keyboard-only and screen-reader users).
- **Tooling**: Playwright for web journeys with `AC-n` ids traced from spec to test (every
  criterion has a test, every test names a criterion), axe-core in the same run blocking on
  serious and critical violations, Schemathesis when an OpenAPI or GraphQL schema exists,
  property-based tests where the language has a library, and mutation testing scoped to the
  diff as advisory evidence of test adequacy. Mobile runs as its own declared profile, since
  emulators need KVM or macOS. Playwright's healer agent may fix locators, never assertions.
- **Agentic user runs**: the product runs inside the no-network sandbox; the persona agent runs
  as a subagent of the owner's host, driving it through Playwright MCP, with no access to the
  source. AgenteARQ still never calls a model. Model version, step cap and budget are fixed,
  traces keep `capture_content: false`, and each finding is reported as "seen in k of n runs".
  It produces findings with reproduction steps, never a pass. A finding becomes a gate only
  after plain Playwright replays it without a model and the owner approves the new test.
  Research shows simulated users are unreliable stand-ins for people, which is why they advise
  and never block.
- **Owner acceptance**: before `pr_ready`, the owner gets a short plain-language script ("try
  these three things") and records accept or reject with a sentence. Rejection returns to
  implementation with the owner's words as the new requirement.

Exit: on the benchmark suite, products built with scenarios and personas show fewer defects
caught by the hidden tests than without them, measured, not asserted.

### Phase 3b — Design that does not look generated (G12)

Generated interfaces converge on the same look (indigo gradients on white, Inter or Roboto, a
centred hero with three identical icon cards, gradient text, the same radius and shadow
everywhere). Ban lists alone fail: each one produces a new default, so the standard is a
process plus deterministic checks, with the ban list versioned as an artifact.

- **Direction first**: before any interface, `design/brief.md` (subject, audience, main task,
  references, the one bold element, vetoed looks) and `design/tokens.json` in the W3C DTCG
  2025.10 format. Platform code is generated from the tokens (Style Dictionary or Terrazzo) and
  never edited by hand.
- **Owner choices with options**: typefaces from open licences with the licence recorded, an
  icon family, and density for the domain.
- **A new standard, `17-design-quality`**, through the RFC process:

  | Check | Mode |
  |---|---|
  | direction declared (`brief.md` and valid `tokens.json`) | `fail_closed` |
  | colour, size, radius and shadow only through tokens (stylelint) | `fail_closed` |
  | WCAG 2 AA contrast on token pairs | `fail_closed` |
  | no serious or critical axe violations on key pages and states | `fail_closed` |
  | visible focus and `prefers-reduced-motion` respected | `fail_closed` |
  | no placeholder copy | `fail_closed` |
  | no horizontal overflow at 375, 768 and 1280 px | `fail_closed` |
  | font licence recorded | `fail_closed` |
  | visual baseline diff, owner approves a new baseline | `fail_warn` |
  | generic-look score from DOM heuristics, skipped when the brief asks for that look | `fail_warn` |
  | APCA contrast | `fail_warn` |
  | screenshot critique by a model, run by the host | `fail_warn`, never the only blocker |

Exit: the pilot's products pass every `fail_closed` design check, and a blind panel cannot pick
them out from human-designed products more often than chance on a fixed sample.

### Phase 4 — Independent review for a solo owner (G5)

- A **reviewer agent** started in a fresh context, read-only, with its own instructions and the
  diff, spec, scenarios and evidence. It records `kind: independent_agent`, which is stated
  everywhere as weaker than a human review.
- A **plain-language review summary** for the owner: what changed, what was checked, what risk
  remains, in five lines.
- Optional human review stays available and is recorded as stronger evidence.

Exit: a solo pilot participant reaches `pr_ready` without fabricating an identity; the reviewer
agent finds the seeded defects in the `99-failing`-style fixtures at a measured rate.

### Phase 5 — Speed and token economy (G9)

- **Context packs**: `get_next_task` returns only the task, its criteria and scenarios, the
  files likely affected and the rules that apply, within a declared size budget.
- **Deterministic summaries**: status, diffs of evidence and refusals come from the CLI, so the
  model never re-reads large artifacts to learn the state.
- **Stable prefixes**: generated instruction files stay within the fixed core budget (ADR 0002)
  so hosts can cache them.
- **Model routing advice**: per step, which tasks suit a smaller model (exploration, persona
  runs) and which need the strongest one (design, review); advice only, the host decides.
- **A small, static MCP tool list**: changing tool definitions invalidates the whole prompt
  cache, so the phase is applied in responses and hooks, never by switching tools. Responses are
  short by default, with `detail` and `cursor` parameters and `resource_link` to sections of the
  standard instead of embedding them.
- Token and wall-time per phase become benchmark metrics, collected through Claude Code's
  OpenTelemetry export tagged per run, with exporter settings from the user's environment, never
  from the repository.

Exit: benchmark reports tokens and time per product per phase for both conditions; the
AgenteARQ condition costs no more than a stated ceiling over the baseline.

### Phase 6 — Any solution on the market (decision support)

- **Decision guides**, not rankings, for the choices lay users face: hosting, database,
  authentication, payments, e-mail, file storage, analytics, AI model provider. Each lists the
  criteria that matter (cost model, lock-in, data residency, effort to operate, free tier) and
  how to gather current facts.
- ADRs for these decisions record the **date and sources** of the facts used and the
  **estimated monthly cost**, which the owner approves.
- **Integration recipes**: secrets by reference, environment templates, and MCP allowlist
  entries pinned to exact versions for any service the assistant connects to.

Exit: every guide is linted for vendor neutrality (no unconditional recommendation); an ADR
without sources or cost estimate for these categories is refused before `planned`.

### Phase 7 — Production and after (G8)

- **Release questions in plain language** that produce `release.md` (where it runs, who owns
  it, how to undo, how we know it works, what we watch).
- **Deploy playbooks** per chosen host, as step lists the assistant walks the owner through;
  AgenteARQ still never deploys.
- **Post-deploy smoke** reuses the Phase 3 scenarios against the production URL, run by the
  owner's CI or host and imported as evidence; attestation remains the fallback.
- **Operating the product**: backups and a restore drill, uptime and error alerts, a monthly
  dependency and security update cycle, and an incident playbook, each a checklist with
  recorded completion.
- **`production_verified` requires** a tested rollback, a dated restore drill, an uptime
  monitor, an error tracker referenced by name only, and the owner's written acknowledgement of
  the chosen free tier's limits (for example, a hobby tier that forbids commercial use, a
  database that expires or pauses when idle).

Exit: three pilot products reach `production_verified` with an imported smoke result and a
completed restore drill.

### Phase 8 — Proof and 1.0 (G10, G11)

- Public benchmark: lay-user scenarios, three hosts, with and without AgenteARQ, reporting the
  share of products reaching production, defects escaping to hidden tests, tokens, wall time,
  number of owner decisions and number of times a human had to intervene.
- Freeze `spec/1.0`, release CLI and content 1.0, and publish the benchmark beside it.
- Claim "best" only for the metrics where the results show it, with the conditions stated.

Exit: benchmark published with raw data; `spec/1.0` frozen; signed 1.0 release.

## Progress

Implemented on branch `feat/roadmap-phases` (2026-11-23), each with tests:

- **Phase 1:** `agentarch doctor`; `agentarch hook` and the Claude Code hooks installed by
  `plugin install claude`; the per-host boundary table in [plugins](plugins.md); a
  plain-language entry, in English and Portuguese, for every workflow refusal, enforced by a
  test, with `agentarch explain workflow.<id>`.
- **Phase 2 (partial):** advisory checks for tables without row level security, service-role
  keys in browser code and always-true Firebase rules. `doctor` detects Podman, Docker and
  Apple `container`.
- **Phase 3:** `scenarios.yaml` and `personas.yaml`, validated when present and advised when
  absent; `workflow accept` for the owner's verdict, bound to the verified content.
- **Phase 3b:** standard `17-design-quality` with its RFC; `design/brief.md` and DTCG
  `design/tokens.json` checked for alias integrity, font licences and WCAG 2 contrast; the
  `agentarch-design-interface` skill.
- **Phase 4:** the read-only `agentarch-reviewer` subagent and the review skill's
  five-line summary for the owner.
- **Phase 5:** a bounded context pack (task, criteria text, scenarios, checks) in
  `get_next_task`; the core routing table compacted so every target fits its budget again.
- **Phase 6:** the `agentarch-choose-services` skill and an ADR "Sources and cost" section,
  reminded while planning.
- **Phase 1, import:** `agentarch workflow import-spec` reads Kiro, Spec Kit and OpenSpec
  requirements into numbered acceptance criteria citing their source line.
- **Phase 7:** recommended release sections for backups and a dated restore, monitoring and
  free-tier limits, reminded at `pr_ready` and after release; the release skill's operations
  checklist.

- **Any language:** `--lang` takes any language tag. English and Portuguese have translated
  rules and messages; any other language gets the English rules plus a directive to speak to
  the owner and write every document in it, and every generated target still fits its budget.
  A plain `agentarch sync` keeps the project's language instead of reverting to English.
- **First dogfood pilot** (a ceramics studio's site, built in Portuguese from `start` to
  `reviewing` with the real sandboxed verifier and two fresh-context reviews). What it found,
  now fixed: a decision naming its option without the heading's parenthetical was refused; the
  ADR template lacked the sources section, so a fresh product started with a warning; four
  advice lines arrived at the first step; missing items and one `start` line stayed in English;
  an older `agentarch` on PATH failed on a newer project with a schema error (`doctor` now
  compares versions); and a literal colour outside the tokens passed every check until a
  reviewer saw it (`design.tokens_only` is now checked on the stylesheets).

- **Second dogfood pilot** (a tip calculator CLI, in Spanish, on the `universal` profile, to
  `reviewing` with its declared checks passing in the sandbox). It found that `universal` built
  a Python image when no base was given, contradicting both the documentation and "technology
  is chosen, never assumed": a Rust or Java product would have failed with missing programs.
  `prepare-verifier` now refuses to guess and lists the official images that provide the
  programs the declared checks run.

- **Questions in the chat, not the terminal.** When an assistant runs agentarch there is no
  terminal prompt: it lists choices (`agentarch workflow profiles --format json`) and takes
  answers as flags. The plugin prompt (1.2.0), the session-start hook and the discovery skill
  tell the assistant to ask the owner in its chat, with its question tool when it has one.
- **Independent review of the branch.** A fresh-context reviewer reproduced bypasses of the
  pre-tool hook (symlinked paths, `bash -c`, wrappers such as `timeout` and `sudo -u`, shell
  writes to generated files, a panic failing open) and defects in spec import, translucent
  contrast and file scanning. All are fixed, each with a regression test.
- **CI, run locally step by step:** 46 of the 48 steps of the `sync-check`, `test`, `examples`
  and `blueprint-tests` jobs pass; the two others need `shellcheck` and the runner's install
  directory, and touch no file this branch changed. A stale check for the pt-BR core's old title
  was fixed. The `product-workflow` job needs the runner's local registry and browser install;
  its isolated verification was exercised instead by the two dogfood pilots.

- **Verification without local Docker:** `workflow ci-template` and `workflow ci-verify` run the
  same isolated checks in the owner's GitHub Actions and report them on the pull request,
  without workflow state and never advancing a phase. Exercised on the first pilot.
- **Standard 17 browser checks in the verifier (opt-in):** `browser_checks: true` builds the
  static-site verifier with a Chromium headless shell and adds a `browser` check (axe, layout at
  375, 768 and 1280 px, focus, reduced motion). It passed on the first pilot and failed a broken
  page, both inside the network-less 1 GB sandbox. It becomes the default in content 1.5.

Still open, and why:

- **Phase 0 and 8:** the benchmark cohort and the lay-user pilot need real model runs, their
  cost and real participants; only the owner can authorise them. The pilot is ready to run:
  [pilot protocol](pilot-protocol.md). CI actions were already
  pinned by commit SHA.
- **Phase 2:** Opengrep with an AgenteARQ-owned rule set, `MAL-*` records, an SBOM per run and
  the CI-attestation path need changes to the verifier images and an ADR choosing the path
  without local Docker. Driving Apple `container` waits until its isolation flags are verified.
- **SAST in the security floor:** wiring Opengrep v2.0.0 (LGPL-2.1, binaries pinned by
  SHA-256) with an AgenteARQ-owned rule set is designed, but testing its rules means running the
  downloaded scanner, which this session's permissions did not allow. It ships when its rules
  are tested against fixtures, opt-in first like the browser checks.
- **Phase 1, fast path:** a change to a shipped product already starts at `implementing` (from
  `production_verified`), skipping brief, specification and ADR; it still needs verification and
  an independent review. Skipping review for "small" changes is deliberately not offered, because
  no deterministic rule can tell a small change from a risky one.
- **Hooks for Gemini CLI:** not planned. Its documentation says it was replaced by Antigravity
  CLI on 2026-06-18; an Antigravity adapter waits until its hook protocol is verified. Codex
  hooks are implemented (`plugin install codex`), denying where Claude Code would ask.
- **Making the new checks blocking:** scenarios, risks and recommended release sections are
  advisory now and are planned to block from content 1.5, after their false-positive rate on
  real projects is measured.

## Definition of done for "ready"

The owner decided on 2026-11-23 that a lay-user pilot is not a release criterion. AgenteARQ is
ready for release when:

- every capability in this roadmap that can be built and verified without outside participants
  or paid model runs is implemented and tested (see Progress);
- the branch passes every CI job that can run without network-only infrastructure, run locally
  step by step from `.github/workflows/ci.yml`;
- the remaining gaps are listed below with the decision or resource each one needs.

The [lay-user pilot](pilot-protocol.md) and the comparative [benchmark](benchmark.md) remain
available as evidence to gather after release; neither blocks it.

## Open decisions for the owner

| Decision | Options | Recommendation |
|---|---|---|
| Verification without local Docker | (a) require Docker or Podman; (b) run the pinned image in the owner's CI and import a signed result; (c) a hosted runner | (b): keeps isolation and needs no new infrastructure from us, at the cost of CI setup that `doctor` must guide |
| Where agentic users run | (a) agent and app both inside the sandbox; (b) app in the no-network sandbox, agent as a host subagent through Playwright MCP | (b): (a) would need network to reach a model and would make AgenteARQ call one, breaking two promises |
| First host to get hooks | Claude Code, Codex, Gemini CLI | Claude Code: largest lay-user base and a mature hook model |
| Pilot audience | internal volunteers; external lay users | external lay users: internal volunteers already know the vocabulary |

## Risks

- **Hooks differ per host.** Enforcement will be uneven; each host's real boundary must be
  documented as it is.
- **Agentic users are expensive and non-deterministic.** Keep them advisory, budget them per run
  and promote only reproduced findings to gates.
- **Recipe sprawl.** Each recipe needs a passing fixture, a failing twin and a maintainer;
  otherwise it is not accepted.
- **Market facts go stale.** Decision guides teach criteria; current prices and limits are
  gathered at decision time and dated in the ADR.
