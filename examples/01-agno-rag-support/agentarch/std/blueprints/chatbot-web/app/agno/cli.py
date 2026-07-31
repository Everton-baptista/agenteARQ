"""The same agent, with no web server anywhere.

    python -m app.cli "how much does the Pro plan cost?"

This file exists to prove something, not for convenience. If the agent core could only run inside
FastAPI, the layout would be decoration; because it cannot import from app/api, the loop, the
guardrails and the tools all work here unchanged. That is what makes them testable, reusable from
a queue worker or a batch job, and portable to whatever replaces your web framework.

The one difference is approval. In a service the approver is not in the request, so the run pauses
and a second request carries the decision. Here the approver is standing at the terminal, so the
prompt is synchronous — same rule, different transport, and the rule lives in the core either way.
"""

from __future__ import annotations

import sys

from .agent import runner
from .agent.manifest import ContractError
from .agent.principal import Principal
from .infra.provider import MissingCredential


def main() -> int:
    question = " ".join(sys.argv[1:]) or "how much does the Pro plan cost?"

    # In a deployment this comes from your auth layer. Here it is explicit and local, which is the
    # honest version — the CLI has no caller to authenticate.
    principal = Principal(tenant_id="local", subject="cli", customer_id="cus-42")

    try:
        outcome = runner.run(question, principal)
    except MissingCredential as err:
        print(err)
        return 1
    except ContractError as err:
        print(err)
        return 2

    if outcome.status == "awaiting_approval":
        preview = outcome.approval or {}
        print("\n─── approval required ───────────────────────────────")
        print(f"  tool      {preview.get('tool')}  ({preview.get('effect')}, cannot be undone)")
        print(f"  approver  {preview.get('approver_role')}")
        print(f"  acting for{preview.get('acting_for')}")
        for k, v in (preview.get("arguments") or {}).items():
            print(f"  {k:9} {v}")
        print(f"  on timeout: {preview.get('on_timeout')}")
        try:
            granted = input("\n  approve? [y/N] ").strip().lower() in ("y", "yes")
        except (EOFError, KeyboardInterrupt):
            granted = False  # on_timeout: deny — an unanswered request is not consent
        if not granted:
            print(f"[ESCALATE] {preview.get('tool')} was not approved")
            return 0
        outcome = runner.run(
            question, principal, resume=outcome.state, approved={outcome.state["tool"]}
        )

    print(outcome.text)
    if outcome.citations:
        print(f"\ncited: {', '.join(outcome.citations)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
