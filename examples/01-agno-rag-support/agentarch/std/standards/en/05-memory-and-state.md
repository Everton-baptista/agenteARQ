# 05. Memory and state

Purpose: what an agent remembers, for how long, and who else can see it.
Version: 0.1 · Status: draft

Memory turns a stateless call into a system with history — and history is an attack surface.
Text an attacker got into memory on Monday is instruction the model reads on Friday, in a
conversation with someone else.

---

## 1. Rules

### control.ai.agent.memory_scoped

**Intent.** One user's memory never reaches another.
**Severity** `blocker`

`memory.kind` and `memory.scope_key` are declared. The scope key is the identity boundary, and
it comes from the session, never from anything the model produced.

| Kind | Lives for | Visible to |
|---|---|---|
| `none` | nothing persists | — |
| `session` | one conversation | that conversation |
| `user` | across conversations | one user |
| `shared` | across users | everyone in scope — the one that needs a reason |

`shared` is where cross-tenant leakage happens. It is occasionally the right answer, and it is
never the right default.

### control.ai.agent.memory_ttl

**Intent.** Memory expires.
**Severity** `major`

`memory.ttl_days`. Memory with no expiry accumulates until it is a dataset nobody chose to
build, under a retention policy nobody wrote.

### control.ai.privacy.memory_no_raw_pii

**Intent.** Long-lived storage does not fill with identifiers.
**Severity** `blocker` where personal data is processed

`memory.pii_allowed` defaults to false. Store a reference, not the value: `customer_id`, not the
email address, the order contents and the complaint.

---

## 2. Memory poisoning

The attack: get text into memory once, have it read as instruction later.

| Vector | Shape |
|---|---|
| User-supplied "remember that…" | the agent stores an instruction as a fact |
| Tool result written to memory | an attacker-controlled field persists |
| Summarisation | injected text survives into the summary as a stated fact |
| Shared memory | one tenant writes, another reads |

The defence is the same as everywhere else in this catalogue: **what comes out of memory is
untrusted content.** It is rendered as data, in a delimited block, on the way back in — not
appended to the instructions because it happens to be short and structured.

---

## 3. Do / don't

| Do | Don't |
|---|---|
| Take the scope key from the session | Derive it from anything the model produced |
| Treat recalled memory as untrusted | Trust it because your system wrote it |
| Set a TTL | Keep memory until storage costs notice |
| Store references | Store the raw value because it is convenient |
| Justify `shared` in writing | Use it as the default because it is simpler |

---

## 4. Observed anti-patterns

**The scope key from the model.** The agent is asked to recall "the account we discussed", and
an injected instruction chooses which one.

**Recalled memory as instruction.** It is short, structured and written by your own system, so
it gets appended to the prompt. Its contents came from a user.

**Memory that never expires.** Two years in, it is a personal-data store nobody assessed and no
retention policy covers.

**Summarisation as sanitisation.** Passing hostile text through a summariser produces a summary
of hostile text, now stated as fact by your own system.

---

## External references

Reviewed 2026-07-28. Mappings only; this standard never reproduces the source text.

- NIST AI Risk Management Framework 1.0 and its Generative AI Profile.
- ISO/IEC 42001:2023, for the management-system obligations these controls evidence.
- OWASP Top 10 for LLM Applications — see `09-security.md` for the full mapping.
