# 06. Planning and multi-agent

Purpose: loops, delegation, and why more agents multiply the questions rather than dividing them.
Version: 0.1 · Status: draft

Every property this catalogue asks of one agent has to hold for a system of agents, and several
get harder. Termination is no longer local. Authority can be transferred by a message. Cost
compounds. Each agent is another place untrusted content enters.

The first question is whether you need more than one. "Specialised agents" often means "one
agent with a longer tool list, split across three prompts" — with three times the surface and no
extra containment.

---

## 1. Rules

### control.ai.agent.loop_guard and control.ai.agent.termination_guaranteed

**Intent.** The system stops.
**Severity** `blocker`

`max_steps` and `max_tool_calls` bound one agent. For a system, the budget has to be **shared**:
three agents with ten steps each is a system with thirty, and a delegation cycle between them is
a system with no bound at all.

Detect the cycle explicitly. A → B → A is not caught by any per-agent limit.

### control.ai.agent.handoff_contract

**Intent.** Delegation is declared, not improvised.
**Severity** `blocker`

Every entry in `handoff.hands_off_to` declares:

| Field | Why |
|---|---|
| `payload_schema` | untyped handoff is not a contract |
| `authority` | `read_only` or `delegated` — never inherited by default |
| `return_point` | where control comes back; without it, delegation is a one-way street |
| `timeout_s` | a hand-off with no deadline is a task that waits forever |

**Authority does not transfer with the message.** The receiving agent gets the tools its own
manifest declares. An agent that inherits the caller's tools has an authority nobody wrote down.

### control.ai.agent.delegation_budget

**Intent.** The combinatorics stay bounded.
**Severity** `major`

Delegation depth and total steps across the system. Agents that can call each other freely
produce cost that is quadratic in a way nobody predicted from reading any single manifest.

---

## 2. Handoff as an injection boundary

Another agent's output is untrusted content. It arrives through a channel you built, from a
component you wrote, which is exactly why it gets trusted — and exactly why it works as a
carrier.

An agent that reads a poisoned document and hands off a summary has laundered the injection: the
receiving agent sees a message from a trusted peer.

Validate the payload against its schema at the boundary, and render the free-text parts of it in
a delimited block like any other untrusted input.

---

## 3. Orchestrator or swarm

| | Orchestrator | Swarm |
|---|---|---|
| Control flow | one agent decides | emergent |
| Termination | one place to bound | every path |
| Debugging | a trace | an archaeology |
| Failure | localised | propagates |

Prefer the orchestrator. A swarm is harder to bound, harder to explain after an incident, and
its flexibility is rarely the property that was actually needed.

---

## 4. Do / don't

| Do | Don't |
|---|---|
| Ask whether one agent would do | Split by role because it reads well on a diagram |
| Share a budget across the system | Give each agent its own and hope |
| Declare authority per handoff | Pass the caller's tools through |
| Validate the payload at the boundary | Trust a peer because you wrote it |
| Detect cycles explicitly | Rely on per-agent step limits |

---

## 5. Observed anti-patterns

**Three agents, one tool list.** The split is presentational; the blast radius is unchanged and
the surface is tripled.

**Authority by inheritance.** The orchestrator can issue refunds, so everything it delegates to
can too.

**The unbounded cycle.** A asks B, B asks A for clarification, both under their own step limits,
neither exceeding it.

**Handoff without a return point.** Work disappears into a specialist that has no way back and
no timeout.

**A peer's output as instruction.** The one place teams reliably forget that untrusted content
is untrusted, because the message came from their own code.

---

## External references

Reviewed 2026-07-28. Mappings only; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
