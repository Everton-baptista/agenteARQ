# Checklists

The same workflows as `agentarch/std/skills/`, written as imperative steps you follow yourself.

Skills are loaded automatically by assistants that support the format. These exist so the
standard does not work better in one tool than in another — a standard that does is not a
standard, and the promise on the front page is that every assistant follows the same rules from
one source of truth.

Use them with Gemini CLI, Copilot, Codex, Grok, Kimi, Qwen Code, a local model, or on your own.
Paste the relevant one into the conversation, or work through it by hand.

| Checklist | Skill it mirrors |
|---|---|
| `new-agent.md` | `agentarch-new-agent` |
| `review-agent.md` | `agentarch-review-agent` |
| `threat-model.md` | `agentarch-threat-model` |
| `eval-bootstrap.md` | `agentarch-eval-bootstrap` |
| `upgrade-review.md` | `agentarch-upgrade-review` |
| `refactor.md` | `agentarch-refactor` |
| `discover-product.md` | `agentarch-discover-product` |
| `specify-product.md` | `agentarch-specify-product` |
| `design-product.md` | `agentarch-design-product` |
| `implement-task.md` | `agentarch-implement-task` |
| `debug-systematically.md` | `agentarch-debug-systematically` |
| `verify-product.md` | `agentarch-verify-product` |
| `review-change.md` | `agentarch-review-change` |
| `prepare-pr.md` | `agentarch-prepare-pr` |
| `release-product.md` | `agentarch-release-product` |
| `design-interface.md` | `agentarch-design-interface` |
| `test-scenarios.md` | `agentarch-test-scenarios` |
| `persona-run.md` | `agentarch-persona-run` |
| `choose-services.md` | `agentarch-choose-services` |

If a checklist and its skill ever disagree, that is a bug — `agentarch validate` reports it as
`AA-SKL-018`.
