# Adding an assistant target

A target is a promise that the standard keeps working in that tool, so adding one needs an RFC.

## What a target is

An entry in `internal/render/targets.go` with a name, a path, a byte budget, optional front
matter, and a description of which assistants read it.

## Deciding the budget

Find the tool's practical limit and set the budget below it. When in doubt, lower: the
consequence of a budget that is too tight is a build failure with a clear message, and the
consequence of one that is too loose is an assistant silently ignoring the end of the file.

Rendering MUST fail rather than truncate. An assistant that receives half a rulebook follows half
the rules, and nobody finds out until something goes wrong.

## Front matter

Some formats need their own header — Cursor's MDC, Windsurf's trigger. It goes before the
agentarch header, and the agentarch header must still carry the core digest, or drift becomes
undetectable for that target.

## Formats that cannot carry a comment

`.mcp.json` is the existing case. It is derived from a project artifact rather than rendered from
the core, so drift is detected by comparing the whole file. If your target is like this, say so in
the RFC — it is a different mechanism, not a variation.

## What to submit

1. An RFC: which assistants read this file, what the limit is, how you measured it.
2. The registry entry.
3. A conformance fixture.
4. A CI assertion that the rendered file lands where that assistant looks.

## What not to do

**Do not add a target you cannot test.** A path no assistant reads is a file the project carries
forever, and the first person to notice will be someone debugging why their rules are not being
followed.

**Do not raise a budget to make a target fit.** The core is shared. If it does not fit the
tightest target, the core is too big — that is the budget doing its job.
