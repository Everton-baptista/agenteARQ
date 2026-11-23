# Documentation

**Direction**

- [Mission and roadmap to 1.0](mission-and-roadmap.md) — what AgenteARQ is for, an honest assessment of where it stands, and the phased plan to close the gaps.
- [Lay-user pilot protocol](pilot-protocol.md) — how non-technical participants are recruited, observed and measured to decide whether AgenteARQ is ready.

**Using AgenteARQ**

- [Standard for AI agents](agent-standard.md) — manifests, the 46 controls, regulatory packs, conformance and the CI gate, for products that contain agents.
- [Product workflow](workflow.md) — phases, evidence, isolated verification, release and production checks.
- [Design quality](../content/standards/en/17-design-quality.md) — a recorded design direction, design tokens and the checks that keep interfaces accessible and distinct from the generic generated look.
- [Host plugins](plugins.md) — installing into Claude Code, Codex and Gemini CLI, and what the plugin can and cannot enforce.
- [Verifier images](verifier-images.md) — building the pinned images verification runs in, including the security floor.
- [Live plugin validation](live-plugin-validation.md) — how the plugins were exercised against real hosts.

**Evidence about AgenteARQ itself**

- [Benchmark protocol](benchmark.md) — how the with/without comparison is run and what it does not claim.
- [Implementation notes](implementation.md) — how the CLI, MCP server and dashboard are built and verified.

**Working on the standard**

- [Rationale](01-rationale.md) — why the standard exists and the choices behind it.
- [Writing guide](02-writing-guide.md) — how standards and controls are written.
- [Authoring a control](03-control-authoring.md) — adding or changing a control.
- [Adding a target](04-adding-a-target.md) — supporting another assistant's instruction file.
- Architecture decisions: [a pack is data, never code](adr/0001-a-pack-is-data-never-code.md) and [the core is a fixed budget](adr/0002-the-core-is-a-fixed-budget.md).
- [RFCs](rfcs/) — proposals for changes to the specification, content or CLI.
