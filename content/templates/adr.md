---
adr: {{NUM}}
title: {{TITLE}}
status: proposed        # proposed | accepted | superseded
date: {{TODAY}}
---

# ADR {{NUM}} — {{TITLE}}

## Context

What situation forces a decision. Include the constraint that makes the obvious answer wrong —
if there is no such constraint, this probably does not need an ADR.

## Options considered

At least two options that genuinely fit, including keeping what exists. For each, what it buys
and what it costs in terms of the context above. An ADR listing one alternative that was
obviously bad is a record of a decision nobody actually made.

### Option A

- Benefits:
- Costs:

### Option B

- Benefits:
- Costs:

## Decision

The option chosen, in the present tense, who chose it, and why it wins here.

## Consequences

What becomes easier, and what becomes harder. The second half is the one worth writing: every
decision has a cost, and the cost is what a future reader needs in order to know when to revisit.

## Sources and cost

Only when the decision is about a third-party service (hosting, database, authentication,
payments, e-mail, storage, analytics, an AI model provider). List each fact the comparison used
with its source URL and the date it was read, since prices and limits change monthly, and the
estimated monthly cost now and at the expected scale. Name secrets the service needs by
reference only, never by value.
