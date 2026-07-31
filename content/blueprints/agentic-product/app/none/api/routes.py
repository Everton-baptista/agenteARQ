"""The routes. Thin on purpose.

A handler here does four things: take a validated request, resolve who is asking, call the agent,
shape the result. It contains no prompt text, no tool logic, no retrieval and no model parameters —
all of that lives in app/agent, which is why the agent can be run without a web server at all.

If a handler starts growing agent logic, the import direction is about to invert and
control.ai.api.core_transport_separated is about to fail. That is the control earning its place:
the drift is gradual and always locally reasonable.
"""

from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Request, status

from ..agent import runner
from ..agent.manifest import ContractError, load_manifest
from ..agent.principal import Principal
from ..infra import telemetry
from ..infra.provider import credential_present
from ..infra.resilience import provider_breaker
from . import approvals
from .deps import current_principal, enforce_caller_budget
from .schemas import AskRequest, AskResponse, ApprovalDecision, Health

router = APIRouter()


@router.get("/health", response_model=Health, tags=["ops"])
async def health() -> Health:
    """Readiness, not liveness.

    It verifies the prompt hash too, because a service whose prompt no longer matches its manifest
    is serving something nobody reviewed — and reporting that as healthy is how it keeps doing so.
    """
    manifest = None
    try:
        manifest = load_manifest()
        prompt_ok, detail = True, ""
    except ContractError as err:
        prompt_ok, detail = False, str(err).splitlines()[0]
    except FileNotFoundError:
        prompt_ok, detail = False, "manifest not found — run from the project root"

    # Which credential has to be present is a consequence of model.provider, so it is read
    # from the manifest rather than assumed. A manifest that will not load leaves it empty,
    # and an empty provider resolves no credential — which is the right answer: a replica
    # that cannot read its own contract is not ready, whatever key happens to be in its
    # environment.
    provider = (manifest or {}).get("model", {}).get("provider", "")
    has_credential = credential_present(provider)

    # The breaker is part of readiness: a replica whose circuit is open will fail every request,
    # and reporting it as healthy is how the load balancer keeps sending it traffic.
    circuit = provider_breaker.state
    ok = prompt_ok and has_credential and circuit != "open"
    return Health(
        status="ok" if ok else "degraded",
        credential_present=has_credential,
        prompt_verified=prompt_ok,
        provider_circuit=circuit,
        pending_approvals=len(approvals.queue),
        tracing=telemetry.ENABLED,
        detail=_why_degraded(prompt_ok, detail, has_credential, provider, circuit),
    )


def _why_degraded(
    prompt_ok: bool, prompt_detail: str, has_credential: bool, provider: str, circuit: str
) -> str:
    """Say which check failed.

    `status: degraded` with an empty detail tells an operator that something is wrong and nothing
    about what — which is a page at 3am that starts with reading the source.
    """
    reasons = []
    if not prompt_ok:
        reasons.append(prompt_detail or "the system prompt does not match the manifest")
    if not has_credential:
        # The provider is named, because "the credential does not resolve" sends an
        # operator to check the key they think is in use, which on a project that switched
        # provider is the wrong one.
        reasons.append(
            f"no credential resolves for model.provider {provider or '(unreadable)'!r}"
            " — see infra/secrets.py"
        )
    if circuit == "open":
        reasons.append("the provider circuit is open; requests are failing fast")
    return "; ".join(reasons)


@router.post(
    "/ask",
    response_model=AskResponse,
    tags=["agent"],
    responses={
        401: {"description": "no or invalid bearer token"},
        429: {"description": "the caller's daily run budget is exhausted"},
    },
)
async def ask(
    body: AskRequest,
    request: Request,
    principal: Principal = Depends(enforce_caller_budget),
) -> AskResponse:
    """Ask the agent to do something.

    Note what is not passed through: nothing from the body reaches the model except `instruction`,
    and it reaches it inside a delimited block. The concatenation invariant 2 forbids would happen
    here if it happened anywhere.

    A 200 does not mean the work was done. `status` says whether it was, was refused, or is waiting
    on a person — and a caller that treats 200 as success will report a pending approval as a
    completed close.
    """
    # For the access log, which records the tenant but never the content.
    request.state.tenant_id = principal.tenant_id

    with telemetry.span(
        "agent.run",
        **{"tenant": principal.tenant_id, "agent.id": "ops-copilot"},
    ):
        outcome = runner.run(body.instruction, principal)

    if outcome.status == "awaiting_approval":
        approval_id = approvals.queue.put(
            approvals.Pending(
                tenant_id=principal.tenant_id,
                subject=principal.subject,
                tool=outcome.state["tool"],
                preview=outcome.approval or {},
                state=outcome.state or {},
            )
        )
        return AskResponse(
            status="awaiting_approval",
            approval_id=approval_id,
            approval=outcome.approval,
            tool_calls=outcome.tool_calls,
            cost_usd=round(outcome.cost_usd, 6),
        )

    return AskResponse(
        status=outcome.status,
        answer=outcome.text,
        tool_calls=outcome.tool_calls,
        cost_usd=round(outcome.cost_usd, 6),
    )


@router.post(
    "/approvals/{approval_id}",
    response_model=AskResponse,
    tags=["agent"],
    responses={404: {"description": "unknown, expired, or belongs to another tenant"}},
)
async def decide(
    approval_id: str,
    body: ApprovalDecision,
    request: Request,
    principal: Principal = Depends(current_principal),
) -> AskResponse:
    """Record a human decision and continue, or stop.

    Deliberately not gated on the run budget: deciding an approval is not starting a run, and
    making a person hit a rate limit to finish something they already started is the kind of
    friction that gets approval switched off entirely.
    """
    pending = approvals.queue.take(
        approval_id, principal.tenant_id, body.decision, principal.subject
    )
    if pending is None:
        # One status for unknown, expired and wrong-tenant. See the note in approvals.Store.take.
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="no such pending approval (it may have expired, which denies by default)",
        )

    request.state.tenant_id = principal.tenant_id

    if body.decision == "deny":
        return AskResponse(
            status="escalated",
            answer=f"[ESCALATE] {pending.tool} was not approved",
            tool_calls=pending.state.get("tool_calls", 0),
            cost_usd=round(pending.state.get("spent_usd", 0.0), 6),
        )

    with telemetry.span(
        "agent.run.resumed",
        **{"tenant": principal.tenant_id, "gen_ai.tool.name": pending.tool},
    ):
        outcome = runner.run(
            pending.state["question"],
            principal,
            resume=pending.state,
            approved={pending.tool},
        )
    return AskResponse(
        status=outcome.status,
        answer=outcome.text,
        tool_calls=outcome.tool_calls,
        cost_usd=round(outcome.cost_usd, 6),
    )
