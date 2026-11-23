# Lay-user pilot protocol

This is the protocol for the pilot that decides whether AgenteARQ meets its mission: a person
with no programming background takes a product of their choice to `production_verified` with
the assistant they already use, without outside help. The roadmap's definition of ready asks for
at least 80% of participants to get there; this protocol produces that number, and the friction
behind it, in a form another team could repeat.

Two dogfood pilots run by the maintainers (a ceramics studio site in Portuguese, a tip
calculator CLI in Spanish) preceded this protocol and are not counted: they were driven by
someone who knows the tool.

## Participants

- Five to eight adults who have never written a program, recruited outside the project. At
  least two use a language other than English, and at least two use a phone as their main
  computer outside the session.
- Each brings their own idea, written in one sentence before the session. Ideas that need money
  to move, health data or an AI model inside the product are steered to a smaller version,
  because those paths need owner decisions the pilot does not test.
- Written consent covers screen and audio recording, what is kept, for how long (90 days), and
  that they may stop at any time. No credentials of theirs are ever typed by the facilitator.

## Setup, per participant

1. A clean machine or user account with Git, Podman or Docker, and the assistant installed and
   signed in on the project's own account (Claude Code is the first host).
2. `agentarch` installed from the release under test; `agentarch version` recorded.
3. An empty folder. Nothing else is prepared: `agentarch doctor` and `agentarch start` are part
   of what is being tested.
4. A hosting account on a free tier that allows commercial use, created by the participant
   during the session if the product reaches release; the facilitator never handles it.

## Session

Up to three sessions of 90 minutes, a week apart, so `agentarch` has to resume a product from
recorded state rather than from a fresh conversation.

The facilitator reads this, and then says nothing about the tool:

> Build what you described, with the assistant, until real people can use it. Think aloud.
> I can't help with the tool. If you would stop here at home, say "I'd stop" and we will note it.

The facilitator intervenes only when the participant says "I'd stop" twice for the same
obstacle, or when something unsafe is about to happen (a real secret pasted, a payment).
Each intervention is logged with its cause and counts against the participant's "unaided"
result.

## What is recorded

Per participant, in `pilot/<id>/` outside the product repository:

| Field | Source |
|---|---|
| Phase reached (`draft` … `production_verified`) | `agentarch workflow status --format json` at the end of each session |
| Unaided (yes or no) and every intervention, with cause | facilitator log |
| Every "I'd stop", with the step and the words shown on screen | facilitator log |
| Every gate refusal and whether its explanation was enough | screen recording, `agentarch explain` use |
| Owner decisions asked of them, and how many came with options and a recommendation | transcript count |
| Checks failed then fixed; review verdicts; owner acceptance | `agentarch/project/evidence/latest.json` |
| Tokens and wall time per phase | the host's usage export (OpenTelemetry for Claude Code) |
| Defects found afterwards by a hidden test suite written from their spec | maintainer, after the pilot |
| Design: `fail_closed` checks passed; blind-panel "generic or not" vote on a screenshot | standard 17 checks; panel of three |

No prompt or response text is stored beyond the screen recording, which is deleted after 90
days (invariant 13).

## Decision

AgenteARQ is ready for 1.0 on this host when, across the cohort:

- at least 80% reach `production_verified` unaided;
- nobody fabricated a reviewer, edited a generated file or bypassed a gate;
- every "I'd stop" that occurred more than once has a fix merged and retested;
- defects escaping to the hidden suite are fewer than in the baseline cohort run without
  AgenteARQ (see [benchmark](benchmark.md));
- no `fail_closed` design check failed on a shipped product.

If a criterion fails, the result is published with the friction list and the pilot is repeated
with new participants after the fixes; a participant never repeats, because they have learnt
the tool.

## Facilitator checklist

- [ ] Consent signed; recording on; `agentarch version` noted.
- [ ] Opening sentence read verbatim; no other guidance.
- [ ] Each intervention and "I'd stop" logged with the words on screen.
- [ ] Status exported at the end of each session.
- [ ] Recording stored in the pilot folder only; deletion date set.
