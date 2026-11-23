---
rfc: 0003
title: Give every user interface a recorded direction and deterministic design checks
status: draft
author: Everton Baptista
created: 2026-11-23
affects: [content, cli]
---

# RFC 0003 — Give every user interface a recorded direction and deterministic design checks

## 1. Problem

An interface generated without a direction converges on the same look: indigo gradients on
white, Inter or Roboto, a centred hero over three identical icon cards, the same radius and
shadow on everything. Products built for very different users become interchangeable, which
costs a non-technical owner credibility with the first person who sees the product. Banning the
look in a prompt fails in a documented way: forbid one default and the model moves to the next
one, as Space Grotesk replaced Inter. Contrast and focus failures ship alongside, because nothing
outside the model checks them.

## 2. Proposed rule

A product with a user interface records its design direction in `design/brief.md` and
`design/tokens.json` (W3C DTCG 2025.10) before any interface code, chosen by the owner among
compared options; interface code uses only tokens, and deterministic design checks run in the
verifier while aesthetic and model judgements only warn.

## 3. How it is verified

Deterministic, offline, in the verifier: the brief has its sections, the tokens file parses as
DTCG, every `fontFamily` token carries `$extensions["dev.agentarch"].license`, and every pair in
the root `$extensions["dev.agentarch"].contrast` meets WCAG 2 AA (4.5:1 text, 3:1 large text and
UI). In the product's own browser tests: no serious or critical automated accessibility
violations, visible focus, reduced motion, no placeholder copy, no overflow at 375, 768 and
1280 px. Tokens-only is a stylesheet lint. Visual baseline, the generic-look score, APCA and a
model's screenshot critique are `fail_warn`.

## 4. Severity and grace period

These are workflow checks, not agent controls, so they add no entry to the control registry.
The token checks enter as advice in `workflow status` first and become required before
`planned` for interface profiles one minor later.

## 5. Cost of adoption

Writing the brief and choosing a direction takes the owner one conversation; the skill
`agentarch-design-interface` produces both files. Projects already using a design system map
their existing tokens into the DTCG file once.

## 6. What happens to existing adopters

Nothing fails on upgrade. Projects with an interface see advice to add the two files; the
requirement starts one minor later and only for interface profiles.

## 7. Alternatives considered

- **A ban list in the prompt.** Fails as described in section 1 and cannot be verified.
- **A model judge as the gate.** Contradicts invariant 12 and is not reproducible.
- **Doing nothing.** Leaves contrast and focus failures to be found by users.

## 8. Prose

`content/standards/en/17-design-quality.md`.
