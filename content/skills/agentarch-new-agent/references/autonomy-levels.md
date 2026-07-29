# Choosing an autonomy level

Autonomy is a property of the **deployment**, not of the model. The same prompt at L1 and at L4
is two different systems with two different threat models.

| Level | The agent may | Choose it when |
|---|---|---|
| `L0_suggest` | produce output only; a human performs every action | the output is advice, or you are still learning what the agent does |
| `L1_act_with_approval` | act, with a human approving each action | actions are consequential and volume is low enough that approval stays meaningful |
| `L2_act_reversible` | act alone where the action can be undone | you can enumerate what is reversible, and everything else is gated |
| `L3_act_irreversible_bounded` | act alone within declared numeric or scope limits | the limits are real numbers in the tool spec, not intentions |
| `L4_autonomous` | act alone, bounded only by budget and stop conditions | rarely, and never as a starting point |

## How to decide

Ask what the worst reachable outcome is if the model is persuaded to act against the user's
interest — not whether the model is good, because a good model that has been convinced is
exactly the case being planned for.

Then pick the lowest level that still makes the agent useful. Raising it later is a deliberate
decision with a revalidation trigger attached. Lowering it after an incident is not a decision,
it is a consequence.

## Approval fatigue

L1 looks safest and is not always. A person approving forty things an hour approves the
forty-first without reading, and the control has become a delay. If volume will be high, L2 with
a narrow `approval.required_when` on the few irreversible tools is a stronger control than L1 on
everything.
