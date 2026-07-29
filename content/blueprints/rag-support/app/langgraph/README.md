# Running this agent

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

python app/agent.py "where is my order BR-77120?"
```

## What the graph changes, and what it does not

Compare this with the framework-free version — `agentarch blueprint add rag-support --framework
none`. The manifest, the tool specs and the prompt are identical. **None of the guarantees move.**

What changes is where they attach:

| Guarantee | Without a framework | Here |
|---|---|---|
| verified prompt | read at startup | in state, so no node can substitute its own |
| input guardrail | before the loop | a node with a conditional edge to escalate |
| action guardrail | before dispatch | inside the tool node, before invoking |
| output guardrail | after the reply | a node after the agent node |
| step bound | a `for` loop | `should_continue`, read from the manifest |

## The two things to keep an eye on

**State is the attachment point.** It is the one thing every node sees. Putting the verified
prompt and the session there means a node cannot quietly use a different one.

**`action_guardrail` does not call the model.** A check that shares the model's context shares its
compromise, and in a graph it is easy to add a node that asks the model whether an action is safe.
Do not.

## Human approval

This blueprint refuses irreversible tools rather than approving them, to keep the example short.
In a real deployment use LangGraph's `interrupt`, which maps directly onto
`approval.required_when`. Set a deadline alongside it: an interrupt with no timeout is a task that
waits forever, and `on_timeout: deny` is what the tool spec asked for.

See `agentarch/std/adapters/langgraph.md`.
