"""The agent loop, with an MCP server on the other side of the tool boundary.

What is different from an agent with tools you wrote: **every tool description is untrusted input.**
It was authored by whoever runs the server, it reaches the model as instruction-shaped text, and it can
change between the day you reviewed it and the day it runs. That is the rug pull, and the defence is in
infra/mcp.py — descriptions hashed at review time and re-checked on connect.

Two consequences show up in this file rather than in the client:

  the connection is checked at startup, not per request. A service that discovers on its first
  customer request that a server changed its descriptions has already started serving.

  a tool result is data, delimited. It arrives from a process somebody else wrote, so it goes back to
  the model as content in a tool_result block and never as a system instruction.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field

from ..infra import mcp, telemetry
from ..infra.provider import create_message
from .guardrails import input_guardrail, output_guardrail
from .manifest import load_manifest
from .principal import Principal


@dataclass
class Outcome:
    status: str                       # "answered" | "escalated"
    text: str = ""
    approval: dict | None = None
    state: dict | None = None
    tool_calls: int = 0
    cost_usd: float = 0.0
    # Which server-side tools actually ran. Returned because "which third-party process did this
    # answer come from" is the question an incident starts with.
    mcp_calls: list[str] = field(default_factory=list)


def servers_for(manifest: dict) -> list[dict]:
    """The allowlisted servers this agent declares, and nothing else.

    `default: deny` is asserted here rather than assumed: an allowlist that does not default to deny is
    a list of suggestions, and starting anyway is how it becomes one.
    """
    allowlist = mcp.load_allowlist()
    if allowlist.get("default") != "deny":
        raise mcp.AllowlistError("allowlist does not default to deny; refusing to start")

    used = set(manifest.get("mcp", {}).get("servers_used", []))
    servers = [s for s in allowlist["servers"] if s["name"] in used]
    if not servers:
        raise mcp.AllowlistError("no allowlisted MCP server is declared for this agent")
    return servers


def run(
    question: str,
    principal: Principal,
    resume: dict | None = None,
    approved: set[str] | None = None,
) -> Outcome:
    manifest = load_manifest()

    ok, reason = input_guardrail(question)
    telemetry.record_guardrail(
        "input", "allow" if ok else "block", "control.ai.genai.prompt_injection"
    )
    if not ok:
        return Outcome(status="escalated", text=f"[ESCALATE] {reason}")

    servers = servers_for(manifest)
    messages = resume["messages"] if resume else [
        # Delimited, because the question is untrusted — and because everything else in this
        # conversation will be too.
        {"role": "user", "content": f"<question>\n{question}\n</question>"}
    ]
    calls = (resume or {}).get("tool_calls", 0)
    spent = (resume or {}).get("spent_usd", 0.0)
    used_tools: list[str] = []

    with mcp.AllowlistedMCPClient(servers[0]) as client:
        with telemetry.span("agent.mcp.handshake", **{"mcp.server": servers[0]["name"]}) as span:
            # Raises if a description changed since review. Deliberately before the first model call:
            # discovering a rug pull after answering is discovering it too late.
            tools = client.tools()
            span.set_attribute("mcp.tools_allowed", len(tools))

        for _ in range(manifest["autonomy"]["max_steps"]):
            with telemetry.span(
                "agent.model_call",
                **{"gen_ai.request.model": manifest["model"]["id"], "tenant": principal.tenant_id},
            ), telemetry.Timer() as timer:
                response = create_message(
                    # The provider is read from the contract and passed down, never read inside
                    # infra — see the note at the top of infra/provider.py. It is the same field
                    # `agentarch check` reads for control.ai.supply.model_pinned, so the manifest
                    # and the call cannot name different providers.
                    provider=manifest["model"]["provider"],
                    model=manifest["model"]["id"],
                    max_tokens=manifest["model"]["params"]["max_output_tokens"],
                    temperature=manifest["model"]["params"]["temperature"],
                    system=manifest["_system_prompt"],
                    tools=tools,
                    messages=messages,
                )

            spent += telemetry.record_model_call(
                model=manifest["model"]["id"],
                tenant=principal.tenant_id,
                input_tokens=response.usage.input_tokens,
                output_tokens=response.usage.output_tokens,
                latency_ms=timer.ms,
            )
            if spent > manifest["autonomy"]["budget"]["usd_per_run"]:
                return Outcome(status="escalated", text="[ESCALATE] run cost budget exhausted",
                               tool_calls=calls, cost_usd=spent, mcp_calls=used_tools)

            uses = [b for b in response.content if b.type == "tool_use"]
            if not uses:
                text = "".join(b.text for b in response.content if b.type == "text")
                ok, reason = output_guardrail(text)
                telemetry.record_guardrail(
                    "output", "allow" if ok else "block", "control.ai.privacy.pii_leakage"
                )
                return Outcome(
                    status="answered" if ok else "escalated",
                    text=text if ok else f"[ESCALATE] {reason}",
                    tool_calls=calls,
                    cost_usd=spent,
                    mcp_calls=used_tools,
                )

            calls += len(uses)
            if calls > manifest["autonomy"]["max_tool_calls"]:
                return Outcome(status="escalated", text="[ESCALATE] tool call budget exhausted",
                               tool_calls=calls, cost_usd=spent, mcp_calls=used_tools)

            messages.append({"role": "assistant", "content": response.content})
            results = []
            for use in uses:
                with telemetry.span(
                    "agent.mcp.tool_call",
                    **{"gen_ai.tool.name": use.name, "mcp.server": servers[0]["name"],
                       "tenant": principal.tenant_id},
                ):
                    out = client.call(use.name, use.input)
                telemetry.record_guardrail(
                    "action", "block" if "error" in out else "allow",
                    "control.ai.mcp.allowlist_enforced",
                )
                if "error" not in out:
                    used_tools.append(use.name)
                results.append(
                    {
                        "type": "tool_result",
                        "tool_use_id": use.id,
                        # json.dumps rather than string interpolation: a server-authored result is
                        # data, and a result that can end a JSON string can start an instruction.
                        "content": json.dumps(out),
                        "is_error": "error" in out,
                    }
                )
            messages.append({"role": "user", "content": results})

    return Outcome(status="escalated", text="[ESCALATE] step budget exhausted",
                   tool_calls=calls, cost_usd=spent, mcp_calls=used_tools)
