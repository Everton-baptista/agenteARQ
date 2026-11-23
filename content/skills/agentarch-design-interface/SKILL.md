---
name: agentarch-design-interface
description: Give a product's user interface a direction the owner chose before any screen is built — propose two or three design directions with trade-offs, record the chosen one in design/brief.md and design/tokens.json, build only with tokens, and critique from screenshots so the result does not look generically AI-generated.
metadata:
  version: "1.0.0"
---

# Design an interface with a direction

Use this for any product with a user interface, before writing interface code. Read
`agentarch/std/standards/17-design-quality.md`; it holds the checks and the current list of
generic-look tells.

1. **Ground it.** Read the brief. Ask the owner, one question at a time, only what is missing:
   who uses it, the one task they come for, two or three products or places they like and why,
   anything they refuse to see. Ground the direction in the domain's own materials and
   vocabulary, not in what interfaces usually look like.
2. **Propose two or three directions.** For each: a name, a one-line idea, palette (four to six
   named colours), two typefaces with roles and licences, density, the one bold element, and what
   it costs (readability, effort, brand fit, licence). Label one as your recommendation. Never
   offer a direction that matches the tells list unless the brief asks for it. Stop and let the
   owner choose; offer the icon family as a separate choice.
3. **Record the choice.** Write `design/brief.md` (subject, audience, main task, references, bold
   element, vetoed looks, chosen direction and who chose it) and `design/tokens.json` in the DTCG
   2025.10 format, starting from `agentarch/std/templates/tokens.json`. Put each font's licence in
   `$extensions["dev.agentarch"].license` and every text/background pair the interface uses in the
   root `$extensions["dev.agentarch"].contrast`.
4. **Generate, do not hand-write.** Produce the platform's variables or theme from the tokens with
   a token build tool the owner approved, and theme any component library away from its defaults.
   Interface code uses tokens only.
5. **Critique.** Take screenshots at 375, 768 and 1280 px. Compare them with the brief and the
   tells list, fix what reads as a default, then remove the one decoration that adds the least.
   A model's critique is advice, never a pass.

Real copy, never lorem ipsum. Visible focus, labels, reduced motion and WCAG 2 AA contrast are
not optional. Stop when the owner has chosen a direction and the tokens validate, or when a
choice that is the owner's is missing.
