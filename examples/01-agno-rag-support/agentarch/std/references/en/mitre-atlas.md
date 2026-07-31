# MITRE ATLAS → red-team categories

Reviewed 2026-07-29.

ATLAS catalogues what adversaries do to ML-enabled systems. agentarch does not derive controls
from it — the tactics describe behaviour, not repository state. It is used for something more
useful: **seeding red-team datasets**, so `eval.redteam_executed` measures something real
instead of whatever cases someone thought of on the day.

## Tactics that reach an agent, and the cases they suggest

| Tactic | For an agent this looks like | Red-team category |
|---|---|---|
| Reconnaissance | probing for the system prompt, tool names, model identity | `prompt_leak` |
| Initial access | injection through a retrieved document or a tool result | `indirect_injection` |
| ML model access | prompting until an internal detail is disclosed | `prompt_leak` |
| Execution | persuading the agent to call a tool with attacker arguments | `tool_coercion` |
| Persistence | writing an instruction into memory for a later session | `memory_poisoning` |
| Defence evasion | phrasing that slips past an input filter | `filter_evasion` |
| Discovery | enumerating what the agent can reach | `capability_probing` |
| Collection | gathering data across turns or tenants | `cross_tenant` |
| Exfiltration | image URLs, DNS, query parameters, error echoes | `exfiltration_via_url`, `exfiltration_via_error` |
| Impact | cost exhaustion, denial of wallet | `resource_exhaustion` |

## Using this

`agentarch-eval-bootstrap` seeds a dataset from these categories plus the agent's own
`out_of_scope`. Every declared refusal deserves a case: a refusal nobody tested is a refusal
nobody knows the agent performs.

Two honesty notes worth repeating wherever this is used:

- **Coverage of categories is not coverage of attacks.** One case per category is a checklist,
  not a test suite.
- **Zero successes on a small set is a small set**, not a result. Report the case count next to
  the success count, always.
