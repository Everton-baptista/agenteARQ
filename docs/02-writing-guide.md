# Writing guide

For anything under `content/`. Read an existing standard before writing a new one; this is the
short version of what you would infer.

## Structure

Numbered sections. `## 1. Rules` first, then do/don't, affected fields, expected evidence,
observed anti-patterns, external references. A reader looking for one thing should be able to
guess which section it is in.

## Register

Sober. No emoji, no superlatives, no exclamation marks. Tables where the content is a mapping;
prose where it is an argument.

Write the **reason**, not only the rule. A rule without its reason gets reimplemented as a
plausible-looking variation, or dropped the first time it is inconvenient. "Set `on_timeout:
deny`" is a rule; "an unanswered approval is not consent" is why it survives review.

## Anti-patterns

Every standard has a section of them, and each must be a **failure mode with a mechanism**, not
a style preference.

Good: *"The prompt that grew. Nobody can say which paragraph does what, so nobody removes
anything, so it keeps growing and the model weighs each instruction less."*

Bad: *"Prompts should be concise."*

Draw them from real incidents where you can. A reader recognises their own system in a specific
failure and skims a general one.

## What not to do

**Do not name a framework** outside `content/adapters/`. `AA-FWK-014` enforces it, and without
the lint the neutral core is fiction within two releases.

**Do not reproduce a source.** OWASP, ATLAS, ISO and the OTel conventions belong to their
authors, change on their schedule, and are authoritative where they live. Map them. The mapping
is the part that is stable and the part nobody else wrote down.

**Do not write prose without a control.** `AA-DOC-008` fails it in both directions. If the rule
cannot be automated, say so with `check.kind: manual_attestation` — an honest declaration, not a
loophole.

**Do not add to the core** without removing something. The budget is fixed; that is the point.

## Translations

English is normative. A translation records the SHA-256 of the source it was made from, and
`validate` reports it when the source moves.

Never make a translation say something the source does not. A stale or divergent translation is
worse than a missing one: a missing one sends the reader to the English, while a wrong one
answers their question with authority.

Control ids, schema fields and file names stay in English in every language.
