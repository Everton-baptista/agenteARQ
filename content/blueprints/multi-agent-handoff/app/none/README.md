# Running this system

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

python app/agent.py "I was charged twice for order BR-77120"
```

## Before you use this

Ask whether you need more than one agent. "Specialised agents" often means one agent with a
longer tool list, split across three prompts — with three times the surface and no extra
containment. Every property this standard asks of one agent has to hold for a system of them,
and several get harder.

## The three things that do not happen by themselves

**The payload is typed.** `validate_payload()` runs at the boundary, because the sender composed
it and the receiver is about to act on it — and one of those two is where an injection would
have landed.

**Authority does not travel with the message.** `run_agent(..., authority=...)` builds the tool
list from the *receiving* agent's manifest, and `read_only` means no tools at all. An
orchestrator that can issue refunds must not make everything it delegates to able to.

**The budget is shared.** `Budget` is created once for the whole system. Three agents with ten
steps each is a system with thirty, and `A → B → A` is caught by `budget.path`, which no
per-agent limit would notice.

## Try breaking it

- Add an `agent_id` to a `hand_off()` call that is not in `hands_off_to`. It is refused: a
  delegation graph that exists only in the code is one no review ever sees.
- Set `max_depth` to 1 and watch a two-level delegation stop.
- Point a specialist back at the orchestrator and watch the cycle detection fire.

## Orchestrator, not swarm

One agent decides. Termination has one place to bound, failures are localised, and after an
incident there is a trace rather than an archaeology. A swarm's flexibility is rarely the
property that was actually needed.
