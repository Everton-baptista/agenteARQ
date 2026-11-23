# 17. Design quality

Purpose: interfaces that look intentional, belong to their product and are accessible, rather
than the generic look a model produces when nobody gives it a direction.
Version: 0.1 · Status: draft · Scope: any product with a user interface (web, mobile, desktop),
`design/brief.md`, `design/tokens.json`.

A model asked for "a modern interface" samples the centre of what it has seen: the same font,
the same gradient, the same three cards. The cause is distributional convergence, not a lack of
taste, so asking for taste in a prompt does not fix it. A list of forbidden looks does not fix it
either: forbid one default and the model moves to the next most likely one. What works is a
direction written down before any interface exists, a small set of tokens the code must use, and
checks that run outside the model.

---

## 1. Rules

### Direction first

No interface code is written before two artifacts exist and are versioned with the product:

| Artifact | Holds |
|---|---|
| `design/brief.md` | subject and domain, audience, the main task a user comes to do, two or three references the owner likes and why, the one bold element this product will be remembered by, and the looks the owner vetoes |
| `design/tokens.json` | the direction as data, in the W3C Design Tokens Community Group format 2025.10: colours, typography, spacing, radius, shadow and motion |

Platform code (CSS variables, Swift, Kotlin, Dart) is **generated from the tokens** and never
edited by hand, for the same reason generated instruction files are not: two editable sources of
one decision have no answer to which one is right.

The verifier reads two AgenteARQ extensions inside `tokens.json`:

- `$extensions["dev.agentarch"].license` on every `fontFamily` token: the font's source and
  licence, such as `OFL-1.1 via Google Fonts`.
- `$extensions["dev.agentarch"].contrast` at the root: the foreground and background pairs the
  interface actually uses, as `[{ "fg": "{color.text}", "bg": "{color.bg}", "use": "text" }]`,
  where `use` is `text`, `large` or `ui`.

### Owner choices, with options

These change how the product is perceived and what it costs, so they are the owner's, proposed
as two or three options with their trade-offs and a labelled recommendation:

- **Typefaces.** At most two families with named roles. Prefer open licences (OFL). A licence
  that forbids redistribution means the font file cannot be committed to a public repository;
  record the licence either way.
- **Icon family.** One family, used consistently, with its licence recorded. Emoji are not icons.
- **Density.** Dense for tools people use all day (dashboards, back offices), airier for
  products people visit occasionally.

### The process

1. **Ground the design in the subject.** Read the brief: the industry, its materials, its
   vernacular, what its users already trust. Two products in different domains should not be able
   to swap interfaces.
2. **Plan a compact token set.** Four to six named colours, typefaces with roles, one spacing
   scale, one layout idea. More tokens than decisions is a sign nothing was decided.
3. **Review the plan against generic defaults.** Compare it with the tells below and with the
   brief. Anything that looks like the default of a library or a model is either justified by the
   brief or replaced.
4. **Build** with tokens only.
5. **Critique from screenshots** at phone, tablet and desktop widths, then **remove one
   accessory**: the decoration that adds the least.

The brief wins over the tells. A brief that asks for a look on the list gets it, and the advisory
check below is skipped for it.

### Generic-look tells (list version 1)

This list is versioned because every generation of models produces new clichés, and an old list
pushes the model straight into the next one.

| Wave | Tells |
|---|---|
| 1 | purple or indigo gradients on white; Inter or Roboto because nobody chose; a centred hero followed by three identical icon cards; gradient text; glassmorphism; the same radius and shadow on everything; emoji used as icons |
| 2 | cream with terracotta; near-black with one acid accent; monospace labels everywhere; ALL-CAPS eyebrows over every heading; an arrow on every call to action; `A · B · C` meta strings |

### Checks

Deterministic checks block. Aesthetic judgement and anything a model decides only warns: a brief
may legitimately ask for a "forbidden" look, and an LLM judge is never the only blocker
(invariant 12).

| Check | What it verifies | Mode |
|---|---|---|
| `design.direction_declared` | `design/brief.md` is complete and `design/tokens.json` is valid DTCG before any interface | `fail_closed` |
| `design.tokens_only` | colour, size, radius and shadow come only from tokens or their variables | `fail_closed` |
| `design.contrast_wcag2` | every declared contrast pair meets WCAG 2 AA: 4.5:1 for text, 3:1 for large text and UI | `fail_closed` |
| `design.a11y_automated` | no serious or critical automated accessibility violations on every key page and state | `fail_closed` |
| `design.focus_and_motion` | visible focus on every interactive element; reduced-motion preference respected | `fail_closed` |
| `design.no_placeholder_copy` | no lorem ipsum, "coming soon" or generic alt text | `fail_closed` |
| `design.responsive` | no horizontal overflow or overlapping fixed elements at 375, 768 and 1280 px | `fail_closed` |
| `design.font_license_recorded` | every `fontFamily` token names its source and licence | `fail_closed` |
| `design.visual_baseline` | screenshot diff against the baseline the owner approved, tied to the verifier image digest | `fail_warn`; the owner approves a new baseline |
| `design.slop_score` | DOM heuristics for the tells above | `fail_warn`; skipped when the brief asks for the look |
| `design.apca_advisory` | APCA contrast Lc 75 or more for body text | `fail_warn` |
| `design.vision_critique` | critique of screenshots by a model, run by the host | `fail_warn`; never the only blocker |

WCAG 2 AA is the enforceable bar. APCA is advisory because the WCAG 3 drafts have left the
contrast method undecided.

---

## 2. What this standard does not claim

- Automated accessibility checks find roughly half of real barriers. Passing them is not WCAG
  conformance, and the evidence must never say it is.
- A low generic-look score is not good design. It only says the common defaults were avoided.
- Taste is not verified. What is verified is that a direction was chosen by the owner, written
  down, used consistently and kept accessible.
