# Checklist: refactor to the standard

Mirrors `agentarch-refactor`. Use it with Gemini CLI, Copilot, Cursor, Codex, Grok, Kimi, Qwen
Code, a local model, or on your own — paste it into the conversation, or work through it by hand.

**The rule the rest of this enforces:** never change behaviour and structure in the same commit.

## 1. Measure first

```bash
agentarch check --profile standard --format json > /tmp/before.json
agentarch check --profile standard --adopt-baseline
```

- [ ] The number written down: ___ controls failing, ___ blockers. A refactor that starts without
      a number ends without one.
- [ ] Baseline recorded. Without it there is no proof later that anything improved rather than
      moved somewhere the gate does not look.

## 2. Cover before changing

- [ ] For the behaviour this slice moves: is it asserted anywhere **today**? If not, assert it
      first, in its own commit, and watch it pass against the unmodified code.
- [ ] No test harness at all? Building one is the first slice, and "tests can run" is its gate.

Refactoring without this is rewriting. The difference shows up in production.

## 3. Order by the gate, not by what looks untidy

```bash
agentarch check --profile standard --explain-resolution
agentarch explain <control.id>
```

- [ ] Blockers first, then majors, then the rest.
- [ ] Directory reorganisation that no control asks for: deferred. The layout that matters is the
      one `standards/16-service-and-edge.md` describes and `AA-DEP-019` checks — the agent core
      must not import the transport. The rest is taste, and taste spends the review budget the
      real findings need.

## 4. Four gates per slice

Every slice, in this order, before moving on:

- [ ] The project's own tests pass.
- [ ] `agentarch check --profile standard` is no worse than `/tmp/before.json`.
- [ ] `agentarch validate` is clean.
- [ ] No secret in the diff:

```bash
git diff --cached --name-only | grep -E '\.env$|credentials|\.pem$' && echo REFUSE
```

`control.ai.api.secrets_not_committed` is a blocker with no grace period, because by the time
anybody notices, the credential is already public. If one appears: stop, remove it from history,
rotate it. Do not refactor around it.

## 5. Close the ratchet

```bash
agentarch check --profile standard --update-baseline
```

- [ ] Run, and the result reported against the number from §1: ___ → ___, ___ blockers closed.

It never adds entries — only removes what now passes. The debt disappears because it was paid, not
because it was forgiven.

## What this does not catch

Four gates catch a regression the tests cover, a control that got worse, and a leaked secret. They
do not catch a behaviour nobody tested, and they do not catch a design that satisfies every control
and still serves the user badly.

- [ ] Where a change might not be faithful: said so and stopped, rather than continuing and
      reporting a green gate.

A green gate on a broken refactor is the most expensive artifact in this standard.
