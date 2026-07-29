# Running this agent

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

python app/agent.py "where is my order BR-77120?"
```

## What to read first

**`agentarch/project/agents/support-triage/prompts/system.v1.md`** — the layered prompt. The
last section is the untrusted block, and the refusal policy above it is the part most prompts
are missing: without it, a model that has decided to refuse improvises the wording, and the
improvisation is where it speculates about what a human might decide instead.

**`dispatch()` in `app/agent.py`** — identity is injected server-side. The tool takes an order
reference; the customer comes from the session. A tool that accepts a `customer_id` argument has
handed identity selection to whoever can write into the model's context, which for a RAG agent
is anyone who can edit a help centre article.

**`action_guardrail()`** — the last checkpoint before something happens, and the one that never
calls the model. A check that shares the model's context shares its compromise.

## Making it yours

1. Replace `retrieve()` with your retriever. Whatever it returns stays untrusted.
2. Update `context.rag.corpus_id` and `corpus_version` in the manifest. Changing the corpus is
   the `rag_corpus_changed` revalidation trigger, and it is why groundedness can fall with
   nothing in the repository to point at.
3. Edit `out_of_scope`, then mirror it into the prompt's refusal section.
4. Replace the tools with yours. Classify `effect` before writing the implementation — it
   determines approval, guardrails and blast radius.

After any of that:

```bash
agentarch validate            # will catch a prompt edited without a version bump
agentarch check --profile standard
```

## Why the gate blocks what it blocks

`agentarch explain <control.id>` answers that for any finding, including what to change.
