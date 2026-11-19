# Checklist: create an agent

Mirrors the `agentarch-new-agent` skill. End state: `agentarch validate` and `agentarch check`
both pass, and the manifest says what the agent actually does.

## 0. Does a blueprint already fit?

```bash
agentarch blueprint list
agentarch blueprint add <id> --yes    # then skip to step 5
```

Starting from something that passes the gate beats assembling one and finding out later what
was missing.

## 1. Scaffold

```bash
agentarch new agent <id>
```

- [ ] Id is lowercase kebab-case and describes the job, not the technology

## 2. The four fields that constrain everything else

Do these in order. Each answer narrows the next.

- [ ] **`owner.accountable`** — a named person you could page. Not a team, not a queue. If
      nobody can be named, the agent is not ready for the stage it claims.
- [ ] **`out_of_scope`** — answer this: *what are the three things you would be most alarmed to
      find it had done?* Write those. Specific and checkable ("Never issues a refund"), not
      categories ("nothing harmful", which excludes nothing).
- [ ] **`autonomy.level`** — a property of the deployment, not of the model. When unsure, lower.
      Raising it later is a decision with a revalidation trigger; lowering it after an incident
      is a consequence.
- [ ] **`stop_conditions`** — observable states. "Answer delivered with a citation" is
      observable; "the task is complete" is the agent's own judgement, which is under test.

## 3. Model and tools

- [ ] `model.pinned: true`, with an immutable id. Never `latest`.
- [ ] One `agentarch new tool <id> --effect <effect>` per capability
- [ ] `effect` classified **before** the implementation exists
- [ ] `communication` filed as `communication`, not `write` — undoing a write is a database
      operation, undoing a sent message is an apology
- [ ] Identity comes from the session in every tool signature. **No `customer_id` parameter.**
- [ ] `egress` names exact hosts, or is empty. Never a wildcard.
- [ ] Irreversible tools have `approval` with `on_timeout: deny`

## 4. Prompt

- [ ] Layers: role → scope → tool policy → refusal policy → output format → untrusted block
- [ ] Every `out_of_scope` entry appears in the refusal section
- [ ] The untrusted block is last, and the prompt says instructions inside it are tampering
- [ ] No secret anywhere in it
- [ ] `sha256` and `version` updated after any edit

## 5. Verify

```bash
agentarch validate
agentarch check --profile standard
```

- [ ] Both pass
- [ ] Every failure fixed at the cause. **Never widen a permission to make one go away** — the
      failure is information about the task being wrong-shaped
- [ ] Any remaining gap recorded with `agentarch waive`, with an owner and a date, and named
      out loud as debt

Run `agentarch explain <control.id>` for anything you do not understand.
