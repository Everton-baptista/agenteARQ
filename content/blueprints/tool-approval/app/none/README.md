# Running this agent

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

python app/agent.py "update the contact email for acc-1 to a@b.com"   # runs alone
python app/agent.py "close the account acc-1"                          # stops for you
```

## The point of L2

`autonomy.level: L2_act_reversible` means the agent acts alone on anything reversible and does
not act alone on anything else. Both halves matter. The first is what makes it useful; the
second is what makes the level meaningful.

Try both commands above. The contact update runs without asking. The close stops and shows you
the arguments, the effect, the approver role and what happens if nobody answers.

## What to read

**`request_approval()`** — an approver who cannot see the arguments is approving the agent's
judgement rather than the action, and the agent's judgement is the thing under question. The
audit record includes *what was shown*, which is the only way an approval record can answer the
question that matters after an incident.

**`action_guardrail()`** — checks the composition the tool schema cannot see: a valid tool,
wired up with `approval: none`, on an agent that acts alone. Try changing `approval: human` to
`none` for `close_account` in the manifest and run `agentarch check`.

**`domain_limits`** in the tool specs — business bounds keep a successful injection cheap. A
tool that can act is worth less to an attacker when the amount it can act on is capped.

## Approval fatigue

The failure mode that defeats every well-designed approval flow. Watch the approval rate: if it
is near 100% and the median decision takes two seconds, the control has become a delay. The fix
is a narrower `approval.required_when`, not a faster dialog.
