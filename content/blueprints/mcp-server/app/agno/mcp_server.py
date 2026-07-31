"""The agent's tools, served to other agents and IDEs over the Model Context Protocol.

Run it:

    python -m app.mcp_server

stdio only, on purpose. A stdio server is spawned by the client that uses it and inherits that
user's identity from the environment — there is no open port to protect. The day this needs a
network transport, the identity stops coming from the environment and starts coming from a
verified credential, exactly the way api/deps.py does it. An unauthenticated MCP port is an
unauthenticated path to every tool the manifest declares.

Three governance properties live here, and app/tests checks each one:

  - the advertised tool list and input schemas come from the reviewed .tool.yaml specs, not
    from decoration — an undeclared tool cannot leak onto the wire, and the schema the client
    validates against is the one a human reviewed;
  - descriptions_hash() is the sha256 a consumer pins in its allowlist. Edit a description
    and the hash changes, which is the rug-pull tripwire mcp-consumer demands, honoured here
    on the serving side;
  - an irreversible tool pauses however it was reached. An MCP call is a transport, not an
    approver: the human decision still goes through the HTTP approval queue.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os

from mcp.server.lowlevel import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

from .agent import tools as tool_impl
from .agent.manifest import load_manifest, load_tools
from .agent.principal import Principal


def load_specs() -> tuple[dict, dict[str, dict]]:
    """The manifest and its tool specs, verified the same way the agent verifies them."""
    manifest = load_manifest()
    return manifest, load_tools(manifest)


def descriptions_hash(specs: dict[str, dict]) -> str:
    """sha256 over the advertised (id, description) pairs, sorted.

    What a consumer pins in its allowlist. A description edited here changes the hash, and
    every well-behaved consumer then refuses this server until a person re-reviews it — the
    tripwire running on the serving side, where it belongs.
    """
    pairs = [[name, spec.get("description_for_model", "")] for name, spec in sorted(specs.items())]
    return hashlib.sha256(json.dumps(pairs).encode()).hexdigest()


def _principal_from_env() -> Principal:
    """stdio has no request to authenticate, so the identity is explicit and local — the same
    honesty as the CLI's principal. The role decides what may even be requested: a caller who
    cannot close accounts should not fill an approver's queue with requests to close them.
    """
    return Principal(
        tenant_id=os.environ.get("AGENT_TENANT", "local"),
        subject=os.environ.get("AGENT_SUBJECT", "mcp-operator"),
        role=os.environ.get("AGENT_ROLE", "support_agent"),
        account_id=os.environ.get("AGENT_ACCOUNT", ""),
    )


def _pending_payload(name: str, spec: dict, args: dict) -> dict:
    """What an irreversible call returns. Parked, never executed.

    The approval queue lives on the HTTP API because the approver is a person with a browser,
    not the calling agent. Returning "done" here would be the one lie that undoes the whole
    approval model: the irreversible action would have run with no human anywhere.
    """
    approval = spec.get("approval") or {}
    return {
        "status": "awaiting_approval",
        "tool": name,
        "effect": spec.get("effect", ""),
        "arguments": args,
        "approver_role": approval.get("approver_role", ""),
        "on_timeout": approval.get("on_timeout", "deny"),
        "decide_at": "POST /v1/approvals/{approval_id} on this service's HTTP API",
    }


def build_server() -> tuple[Server, dict[str, dict], dict]:
    manifest, specs = load_specs()
    principal = _principal_from_env()
    autonomy = manifest["autonomy"]["level"]

    server = Server(
        "tool-gateway",
        instructions=(
            "Governed tools for the tool-gateway agent. Irreversible tools return "
            "awaiting_approval and a human decides out of band. Pin tool_descriptions_hash "
            f"in your allowlist: {descriptions_hash(specs)} — if it changes, stop and re-review."
        ),
    )

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        # From the specs, so the wire cannot advertise anything a reviewer did not sign off.
        return [
            Tool(
                name=name,
                description=spec.get("description_for_model", ""),
                inputSchema=spec.get("input_schema", {"type": "object"}),
            )
            for name, spec in sorted(specs.items())
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict) -> list[TextContent]:
        spec = specs.get(name)
        if spec is None:
            # An allowlisted client asks by name; anything else gets the same refusal the
            # action guardrail gives, shaped as data rather than a protocol error.
            result = {"error": f"no declared tool {name!r}", "retryable": False}
        else:
            try:
                result = tool_impl.dispatch(name, arguments, spec, principal, autonomy)
            except tool_impl.ApprovalRequired:
                result = _pending_payload(name, spec, arguments)
        return [TextContent(type="text", text=json.dumps(result))]

    return server, specs, manifest


async def _serve() -> None:
    server, _, _ = build_server()
    async with stdio_server() as (read, write):
        await server.run(read, write, server.create_initialization_options())


def main() -> None:
    asyncio.run(_serve())


if __name__ == "__main__":
    main()
