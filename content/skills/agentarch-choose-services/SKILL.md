---
name: agentarch-choose-services
description: Help the owner choose third-party services — hosting, database, authentication, payments, e-mail, file storage, analytics, AI model provider — by comparing options against the brief's constraints with current, dated facts, and record the choice and its monthly cost in an ADR. Never ranks vendors or picks one silently.
metadata:
  version: "1.0.0"
---

# Choose services with the owner

Run this whenever the product needs something it will not build itself. The choice is the
owner's: it changes cost, data location, vendor relationships and what the team must operate.

1. **Start from the constraints** in the brief: budget, where users and data are, privacy and
   regulation, who will operate it and what they know, expected traffic, any mandated or excluded
   vendor. Include "no new service" whenever it can work.
2. **Compare two to four options by criteria, not by popularity:**
   - cost model: free tier, what is charged and at which threshold, what happens when a limit is
     hit (sleeping, pausing, deleting, overage);
   - lock-in and the exit path: data export, open standards, migration effort;
   - data residency and compliance;
   - effort to operate: backups, updates, monitoring, who gets paged;
   - free-tier traps: tiers that forbid commercial use, databases that expire or pause when idle,
     limits that pause every project on the account.
3. **Gather current facts at decision time.** Prices and limits change monthly. Read the
   vendor's own pricing and terms pages now, note the date and the URL of each fact, and say when
   a fact comes only from a third party.
4. **Recommend and stop.** Label one option as your recommendation and why, then let the owner
   choose.
5. **Record it** in an ADR: context, options with benefits and costs, decision and who made it,
   consequences, the **sources with their dates**, and the **estimated monthly cost** now and at
   the expected scale. Secrets for the chosen service are referenced by name only, and any MCP
   server for it is added to the allowlist pinned to an exact version.

Never create an account, accept terms, enter payment details or buy anything; those are the
owner's actions. Stop when the ADR is recorded or the owner needs to decide.
