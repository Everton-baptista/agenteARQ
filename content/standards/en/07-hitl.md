# 07. Human in the loop

Purpose: when a person decides, and what they need in order to decide.
Version: 0.1 · Status: draft

Approval is a control only when the person approving can tell what they are approving. A dialog
saying "the agent wants to run refund_order — approve?" produces a click, not a decision, and
after the fortieth one it produces a click without reading.

---

## 1. Rules

### control.ai.agent.hitl_matrix

**Intent.** Approval is decided by risk, not by intuition.
**Severity** `blocker`

| Autonomy | `read` | `write` | `irreversible` / `money` / `communication` |
|---|---|---|---|
| L0 | human performs everything | — | — |
| L1 | approve each action | approve each action | approve each action |
| L2 | no approval | no approval | **approval required** |
| L3 | no approval | no approval | approval above declared bounds |
| L4 | no approval | no approval | approval only where declared |

The row that matters is L2: reversible actions run alone, irreversible ones do not. That is what
the level means.

### control.ai.agent.approval_context_complete

**Intent.** The approver can actually judge.
**Severity** `blocker`

The request shows: which tool, the full arguments, the effect classification, what triggered it,
what happens on timeout, and — where money moves — the amount and the recipient.

The reason this is a control and not a UI note: an approver who cannot see the arguments is
approving the agent's judgement, not the action, and the agent's judgement is the thing under
question.

### control.ai.agent.approval_timeout_safe_default

**Intent.** Silence is not consent.
**Severity** `blocker`

`approval.timeout_s` and `on_timeout`, which is `deny` or `escalate`. Never `allow`. A system
that proceeds when nobody answered has an approval step, not an approval control.

---

## 2. Approval fatigue

The failure mode that defeats every well-designed approval flow. A person asked to approve forty
things an hour approves the forty-first without reading, and the control has become a delay.

| Symptom | What it means |
|---|---|
| Approval rate near 100% | it is a formality |
| Median decision under two seconds | nobody is reading |
| One person approving everything | there is no second opinion |
| Volume rising with usage | the threshold is wrong |

The fix is a narrower trigger, not a faster dialog. `approval.required_when` is an expression
precisely so that the common, small, safe case can proceed while the unusual one stops.

---

## 3. Do / don't

| Do | Don't |
|---|---|
| Show the arguments | Show the tool name and a button |
| Use `required_when` to narrow the trigger | Require approval for everything and watch it decay |
| Deny on timeout | Allow, "to avoid blocking the queue" |
| Record who approved and what they saw | Record only that approval happened |
| Track approval rate and decision time | Assume the control works because it exists |
| Let the approver reject with a reason | Offer only approve and ignore |

---

## 4. Expected evidence

The audit record: who, when, what they were shown, the decision, the reason if rejected. Without
what-they-were-shown, an approval record cannot answer the only question that matters after an
incident.

---

## 5. Observed anti-patterns

**Approve or ignore.** No rejection path, so the queue fills with things nobody will approve and
nobody has declined.

**Allow on timeout.** Introduced during an incident when the queue backed up, and never reverted.

**The rubber stamp.** One person, forty an hour, 100% approval. The metric was never looked at.

**Arguments hidden behind a details link.** Nobody expands it, and the summary is written by the
component being reviewed.

---

## External references

Reviewed 2026-07-28. Mappings only; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
