# Trademark policy

The name **agentarch**, and the phrase **`agentarch spec/1.0` compliant**, name this project and
this specification. This document says how they may be used.

It exists because the licences deliberately do not answer this. The code is Apache-2.0 and the
spec and content are CC BY 4.0, which means anyone may fork, translate, reimplement and sell.
That is the point: `GOVERNANCE.md` offers it as the structural answer to a project with one
maintainer. But a compliance claim that anybody may make about anything is worth nothing to the
person reading it, and that person is the reason the claim exists.

So: the licences govern the work, and this document governs only the name.

## What you may do without asking

**State a factual claim of compliance.** If your implementation meets
`spec/normative/07-conformance-levels.md` Part 2, say so:

> conforms to agentarch spec/1.0

**Publish a conformance level.** If `agentarch conformance` reports L1, L2 or L3 for your
project, say so — including the badge. An L3 badge carries an expiry and stops being true on its
own; displaying an expired one is a false claim rather than a stale one.

**Refer to the project.** In documentation, articles, talks, comparisons, course material and
research. No permission, no attribution beyond what CC BY 4.0 already requires.

**Fork, translate and reimplement**, under the licences, and say what your work is derived from:

> a fork of agentarch · an independent implementation of the agentarch specification ·
> a Japanese translation of the agentarch standards

## What needs permission

**Naming a product, service or company after it.** `agentarch-pro`, `AgentArch Cloud`,
`agentarch.io` — anything a reader could mistake for the project itself, or for something the
project runs. Ask; the answer is often yes, and a name that distinguishes the two is usually
enough.

**Implying endorsement, certification or affiliation.** "Certified by agentarch", "official
agentarch partner", "agentarch-approved". Nothing here certifies anyone. There is no
certification body, no audit programme, and no vendor relationship, and inventing one in a
product page is the failure this document exists to prevent.

**Using the name for a paid compliance or audit service** in a way that suggests the assessment
comes from this project rather than from you.

## What is never permitted

**Claiming compliance you have not got.** Specifically, per
`spec/normative/07-conformance-levels.md`:

- claiming `spec/1.0` while extending the expression language, adding a check kind that executes
  anything, or relaxing a MUST
- reporting a conformance level higher than the one computed, or shipping an option that does
- displaying an expired L3 badge as current

These are the ones the specification already names as MUST NOT. They are repeated here because
this is where someone looks before putting a badge on a page.

**Modifying the standard and calling the result agentarch.** Change the rules and the name has
to change with them, or two incompatible things are called the same, and a reader cannot tell
which one a claim refers to. Call it what it is: "based on agentarch", "a fork of agentarch".

## The test

Would a reasonable person seeing your use think this project produced, endorsed or vouched for
your work?

- **No** — you do not need permission.
- **Yes** — you do.
- **Unsure** — ask. Opening an issue is enough.

## Enforcement

This is not a threat of litigation. If a use is a problem, the first step is an email or an
issue, and the second is usually a rename.

The only use that will be pursued without that courtesy is a false claim of compliance or
certification, because that one is not aimed at this project. It is aimed at whoever is relying
on the claim.

## Asking

Open an issue on the repository, or contact the maintainer named in `GOVERNANCE.md`.

---

*This policy covers the name only. The code remains Apache-2.0 ([LICENSE](LICENSE)); the spec and
content remain CC BY 4.0 ([LICENSE-CONTENT](LICENSE-CONTENT)). Nothing here restricts a right
either licence grants.*
