# Deciding each manifest field

Only the fields where the right answer is not obvious.

## `purpose`
One sentence: what it does and for whom. If it needs two sentences joined by "and", it is
probably two agents, and splitting them now is cheaper than splitting them later.

## `out_of_scope`
The field that does the most work in the whole manifest. Prompt for it with: *"what are the
three things you would be most alarmed to find it had done?"*

Good entries are specific and checkable: "Never issues a refund or credit". Bad entries are
categories: "Nothing harmful" — which excludes nothing, because nobody sets out to do harm.

## `system_type`
Drives which packs apply.

| Type | Use when |
|---|---|
| `generative_chat` | converses, no retrieval, no tools |
| `generative_rag` | answers from a corpus |
| `agentic_task` | one bounded job with tools |
| `agentic_workflow` | multi-step, tools, a plan |
| `multi_agent` | delegates to other agents |
| `classifier` | assigns labels |

## `stage`
`prototype` → `internal` → `pilot` → `production`. Some controls only apply from a stage up, so
claiming `production` early makes the gate stricter, not more impressive.

## `jurisdictions`
Which `reg.*` packs apply. Where the **users** are, not where the servers are.

## `autonomy.budget`
Derive from an observed p95 once something has run. Before that, set a number you would be
willing to be alerted on — a budget nobody would act on is not a bound.

## `context.rag.corpus_version`
Bump whenever the index is rebuilt. This is the `rag_corpus_changed` revalidation trigger, and
skipping it is how groundedness falls with nothing in the repository to point at.

## `privacy.processes_personal_data`
True if the agent ever sees a name, an email, an order, a ticket or an identifier. Almost every
customer-facing agent does, and saying false to avoid the follow-up fields is the failure this
field exists to catch.
