"""Agent consuming MCP servers, with the allowlist as the boundary.

An MCP server contributes text the model treats as authoritative: tool names, descriptions,
parameter documentation. Connecting one is closer to importing a dependency that can write your
prompt than to configuring a client — and the protocol attaches no version to a description, so
nothing in it notices when one changes.

Run it with:  python app/agent.py "what does the deployment guide say about rollbacks?"
"""

from __future__ import annotations

import hashlib
import json
import os
import pathlib
import subprocess
import sys

import yaml
from anthropic import Anthropic

AGENT_DIR = pathlib.Path("agentarch/project/agents/docs-assistant")
ALLOWLIST = pathlib.Path("agentarch/project/mcp/allowlist.yaml")


def description_digest(text: str) -> str:
    """Whitespace is normalised, nothing else. A reflowed description is not tampering; a
    changed word is a changed instruction to the model."""
    return hashlib.sha256(" ".join(text.split()).encode()).hexdigest()


class AllowlistedMCPClient:
    """Talks to an MCP server through the allowlist rather than around it."""

    def __init__(self, server: dict):
        self.server = server
        self.proc: subprocess.Popen | None = None
        self._id = 0

    def __enter__(self):
        s = self.server

        # Only the variables the allowlist names. A server that inherits the process
        # environment inherits every credential in it, and it never had to ask.
        env = {"PATH": os.environ.get("PATH", "")}
        for key in s.get("env_allow", []):
            if key in os.environ:
                env[key] = os.environ[key]

        self.proc = subprocess.Popen(
            [s["command"], *s.get("args", [])],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, env=env, text=True, bufsize=1)

        self._rpc("initialize", {
            "protocolVersion": "2024-11-05", "capabilities": {},
            "clientInfo": {"name": "agentarch-blueprint", "version": "1"}})
        self._notify("notifications/initialized")
        return self

    def __exit__(self, *_):
        if self.proc:
            self.proc.terminate()

    def _rpc(self, method: str, params: dict) -> dict:
        self._id += 1
        self.proc.stdin.write(json.dumps(
            {"jsonrpc": "2.0", "id": self._id, "method": method, "params": params}) + "\n")
        while True:
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError(f"{self.server['name']} closed the connection")
            msg = json.loads(line)
            if msg.get("id") == self._id:
                return msg.get("result", {})

    def _notify(self, method: str):
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method}) + "\n")

    def tools(self) -> list[dict]:
        """List the server's tools, keeping only what was allowlisted and verifying that each
        description still says what it said at review time.

        This is where a rug pull is caught: a server can serve a benign description while it is
        being reviewed and a hostile one afterwards, and nothing else in the protocol would
        notice.
        """
        s = self.server
        allowed = set(s.get("tools_allow", []))
        digests = s.get("tool_description_sha256", {})
        out = []

        for tool in self._rpc("tools/list", {}).get("tools", []):
            name = tool["name"]

            if name not in allowed:
                # Not an incident on its own, but it is the shape silent capability growth
                # takes. It is refused until someone reviews it.
                print(f"  refused  {s['name']}/{name}: not in tools_allow")
                continue

            recorded = digests.get(name)
            if recorded and description_digest(tool.get("description", "")) != recorded:
                raise SystemExit(
                    f"\nSTOP: {s['name']}/{name} changed its description since it was reviewed.\n"
                    f"  approved {recorded[:12]}…\n"
                    f"  serving  {description_digest(tool.get('description', ''))[:12]}…\n\n"
                    "Read the new description before accepting it. Then run\n"
                    "`agentarch mcp audit --probe --record` to re-record the digest."
                )

            print(f"  allowed  {s['name']}/{name}")
            out.append({"name": f"{s['name']}__{name}",
                        "description": tool.get("description", ""),
                        "input_schema": tool.get("inputSchema", {"type": "object"})})
        return out

    def call(self, name: str, args: dict) -> dict:
        bare = name.split("__", 1)[1]
        if bare not in set(self.server.get("tools_allow", [])):
            return {"error": f"{bare} is not allowlisted", "retryable": False}
        return self._rpc("tools/call", {"name": bare, "arguments": args})


def main(question: str) -> str:
    manifest = yaml.safe_load((AGENT_DIR / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]
    raw = (AGENT_DIR / spec["path"]).read_bytes()
    if hashlib.sha256(raw).hexdigest() != spec["sha256"]:
        raise SystemExit("system prompt changed without a version bump")

    allowlist = yaml.safe_load(ALLOWLIST.read_text())
    if allowlist.get("default") != "deny":
        raise SystemExit("allowlist does not default to deny; refusing to start")

    used = set(manifest.get("mcp", {}).get("servers_used", []))
    servers = [s for s in allowlist["servers"] if s["name"] in used]
    if not servers:
        raise SystemExit("no allowlisted MCP server is declared for this agent")

    print("connecting to allowlisted servers:")
    with AllowlistedMCPClient(servers[0]) as client:
        tools = client.tools()

        anthropic = Anthropic()
        messages = [{"role": "user", "content": f"<question>\n{question}\n</question>"}]

        for _ in range(manifest["autonomy"]["max_steps"]):
            response = anthropic.messages.create(
                model=manifest["model"]["id"],
                max_tokens=manifest["model"]["params"]["max_output_tokens"],
                system=raw.decode(), tools=tools, messages=messages)

            uses = [b for b in response.content if b.type == "tool_use"]
            if not uses:
                return "".join(b.text for b in response.content if b.type == "text")

            messages.append({"role": "assistant", "content": response.content})
            messages.append({"role": "user", "content": [
                {"type": "tool_result", "tool_use_id": u.id,
                 "content": json.dumps(client.call(u.name, u.input))} for u in uses]})

    return "[ESCALATE] step budget exhausted"


if __name__ == "__main__":
    if not os.getenv("ANTHROPIC_API_KEY"):
        raise SystemExit("set ANTHROPIC_API_KEY")
    print(main(" ".join(sys.argv[1:]) or "what does the documentation say about rollbacks?"))
