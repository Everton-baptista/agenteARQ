"""The wire contract. These models are the source of contracts/openapi.json.

Two things are deliberately absent from every request model, and their absence is the security
property rather than an oversight:

  - no tenant_id, customer_id or subject. Identity comes from the verified credential in deps.py.
    A request field that names the customer is a request field that selects the customer.
  - no agent_id and no authority. The caller talks to the orchestrator; which specialist is reached is
    decided by the manifest's declared contracts. A request that could name an agent could bypass the
    orchestrator's triage, and one that could name an authority could grant itself any.
  - no model id, temperature, system prompt or max_steps. Those are declared in the manifest and
    pinned. A caller who can raise max_steps can raise your bill; a caller who can send a system
    prompt is a caller who owns your agent.

A field is added here only when the caller must be able to choose it, which is a much shorter list
than it first appears.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field


class AskRequest(BaseModel):
    question: str = Field(
        min_length=1,
        max_length=4000,
        description="The end user's message. Untrusted: treated as data, never as instruction.",
    )
    conversation_id: str | None = Field(
        default=None,
        max_length=64,
        description="Groups turns together. Scoped to the caller's tenant server-side.",
    )

    model_config = {
        "json_schema_extra": {
            "examples": [{"question": "where is my order BR-77120?"}]
        }
    }


class BudgetReport(BaseModel):
    """What the request actually consumed, across every agent.

    Returned because a multi-agent system that reports only its answer gives an operator no way to see
    that one request quietly used nine steps across three agents — and `path` is what answers "how did
    it get there" when a request went somewhere unexpected.
    """

    steps_used: int
    max_steps: int
    depth: int
    path: list[str] = Field(description="The delegation path taken, in order.")
    spent_usd: float
    max_usd: float


class AskResponse(BaseModel):
    status: Literal["answered", "escalated", "awaiting_approval"] = Field(
        description=(
            "answered: the orchestrator produced a reply. "
            "escalated: refused, or a shared bound was reached — including a delegation cycle. "
            "awaiting_approval: an irreversible action needs a human decision."
        )
    )
    answer: str = Field(default="", description="Empty unless status is answered or escalated.")
    budget: BudgetReport | None = Field(
        default=None, description="Shared across every agent in this request, not per agent."
    )
    approval_id: str | None = Field(
        default=None, description="Present when status is awaiting_approval. Decide at /v1/approvals/{id}."
    )
    approval: dict | None = Field(
        default=None,
        description="What the approver must see: the effect, who it affects, and the content.",
    )
    tool_calls: int = Field(default=0, description="Counted against the manifest's max_tool_calls.")
    cost_usd: float = Field(
        default=0.0,
        description=(
            "Estimated spend for this turn, from the same figure the cost metric records — so the "
            "bill and the declared budget cannot disagree."
        ),
    )


class ApprovalDecision(BaseModel):
    decision: Literal["approve", "deny"]
    reason: str = Field(
        default="",
        max_length=500,
        description="Recorded on the audit trail. Required by policy for a denial in most teams.",
    )


class Health(BaseModel):
    status: Literal["ok", "degraded"]
    credential_present: bool
    prompt_verified: bool
    provider_circuit: Literal["closed", "half_open", "open"] = "closed"
    pending_approvals: int = 0
    tracing: bool = False
    detail: str = ""
