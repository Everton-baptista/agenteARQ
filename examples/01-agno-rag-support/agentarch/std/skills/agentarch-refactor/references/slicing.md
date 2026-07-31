# Cutting the work into slices

A slice is a change you can land, verify and explain on its own. If describing it needs the word
"and", it is two.

## The cut that matters most

`standards/16-service-and-edge.md` puts the agent core and the transport in separate layers, and
`AA-DEP-019` fails when `agent/` imports `api/`. In an existing codebase the violation is almost
never a single import — it is a request object threaded four levels down, because at every step
that was the smallest change.

Cut it in this order, and each of these is one slice:

1. **Find the boundary.** `grep -rn "request\|Request" <core dir>` and list what actually crosses.
   Do not move anything yet. Write the list down; it is the plan for the next three slices.
2. **Replace the request with a value.** For each crossing, the core needs one field, not the
   object. Pass the field. The signature grows; that is the point — it now says what it depends on.
3. **Resolve identity at the edge.** The caller becomes a value the transport builds and the core
   receives. `standards/16` §caller and the `Principal` in the shipped blueprints show the shape:
   frozen, resolved server-side, never read from the request body.
4. **Move the files.** Only now, when nothing crosses, does the directory move become mechanical.

Doing 4 first is the usual instinct and it is the expensive order: the imports break everywhere at
once, and there is no intermediate state that runs.

## When a slice will not fit

Some changes have no small version. A synchronous human approval that has to become a queue with a
TTL is the common one — half of it is broken in a way the original was not.

Two honest options:

- **Land it behind a flag**, both paths present, tests for both, and delete the old one in a later
  slice. Costs a slice, buys a working system at every point.
- **Say it is a rewrite of that component**, scope it, and treat it as its own piece of work with
  its own tests. Do not call it a refactor; the word implies behaviour is preserved, and here it
  is not.

What is not an option is a large slice with an optimistic commit message.

## Order within a slice

1. Test for the current behaviour, passing against unmodified code.
2. The change.
3. The test still passes, or the test changed **and you can say why in one sentence**.

Step 3 is where refactors quietly become rewrites. A test that had to be edited to keep passing is
evidence that behaviour moved. Sometimes that is correct and intended — the point is that it is a
decision, made out loud, and not a step in a cleanup.

## What to leave alone

- Naming that is only unfashionable. Renaming touches every call site and hides real changes in
  the diff.
- Code with no test and no failing control. You cannot verify the change, and nothing is asking
  for it. Note it and move on.
- Anything the baseline accepted that no control ranks. The ratchet exists so that debt can be
  deferred honestly; reopening all of it at once is how a refactor stops landing.
