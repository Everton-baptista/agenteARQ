# Adapter: LlamaIndex

Retrieval-first, so the RAG controls carry most of the weight. Corpus versioning is the one most often skipped and the one that invalidates evals silently.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

`ChatPromptTemplate` and system messages. Verify the hash before constructing the query engine.

## 2. Tools and the permission check

`FunctionTool.from_defaults`. Generate the description from the `.tool.yaml`.

## 3. The three guardrail points

Node postprocessors are a natural place for retrieval-side checks. Wrap the query engine for
input and output, and the tool for the action point.

Retrieved nodes are untrusted content: render them into a delimited block, and never let a node
whose text was authored by a third party reach the instruction section.

## 4. Telemetry

LlamaIndex has callback handlers; bridge to OTel and emit a retrieval span with `top_k` and
the corpus version, so a groundedness regression can be tied to a corpus change.

## 5. Handoff and approval

Record `corpus_version` in the manifest and bump it whenever the index is rebuilt. This is the
`rag_corpus_changed` revalidation trigger, and skipping it is how groundedness falls with
nothing in the repository to point at.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
