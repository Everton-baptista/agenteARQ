---
rfc: 0000
title: <short imperative title>
status: draft        # draft | accepted | rejected | withdrawn | superseded
author: <name>
created: YYYY-MM-DD
affects: []          # spec, content, cli
---

# RFC 0000 — <title>

## 1. Problem

What goes wrong today. Concrete, with an example of the failure — not "we should have a control
for X". If you cannot describe something breaking, the control probably does not need to exist.

## 2. Proposed rule

State it as a rule, in one or two sentences, the way it would appear in a standard.

## 3. How it is verified

The most important section. Either:

- an expression, in the restricted language, plus the artifact and fields it reads; or
- `check.kind: manual_attestation`, with an explanation of why it cannot be automated.

Prose with no verifiable consequence does not enter the standard. If the answer here is "a
reviewer notices", say so — that is an honest `manual_attestation`, not a failed RFC.

## 4. Severity and grace period

Proposed severity, and the `enforced_from` version. Remember that no control is born blocking:
a rule that fails builds on the day it ships gets the gate switched off rather than getting
anything fixed.

## 5. Cost of adoption

What a project that already uses agentarch has to do. Estimate the work honestly — a control
that takes a week per agent to satisfy needs a longer grace period than one that takes a minute.

## 6. What happens to existing adopters

Who starts failing, when, and what the upgrade path is. This section is skipped most often and
matters most: every control ships onto systems already in production.

## 7. Alternatives considered

Including doing nothing. Say why the status quo is not good enough.

## 8. Prose

Draft the standard section, or name the existing one it belongs in. A control without prose
fails `AA-DOC-008`, and prose without a control fails it too.
