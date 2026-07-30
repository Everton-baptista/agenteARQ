"""The MCP client that refuses to exceed its allowlist.

An MCP server is a process somebody else wrote, so it lives in infra/ — it is a dependency, not part
of the agent. Putting it here means `agentarch mcp audit` and the tests can exercise it with no model
and no web server in the way.

Three defences, and the third is the one nothing else in the ecosystem standardises:

  default deny        a tool absent from `tools_allow` is not callable, whatever the server offers.
                      The allowlist is the contract; what the server advertises is a proposal.
  pinned version      the server is pinned to an exact version. `latest` means the code you reviewed
                      and the code you run are different artifacts with the same name.
  description hashes  every allowlisted tool's description is hashed at review time and re-checked on
                      connect. A server that changes a tool's description after approval has changed
                      what the model will do with it — the rug pull — and this is the only thing that
                      notices.

The last one is why the digest normalises whitespace only. Normalising more would let a meaningful
rewording pass; normalising less would fail on a reflowed line and train people to re-hash without
reading.
"""

from __future__ import annotations

import hashlib
import json
import os
import pathlib
import subprocess

import logging

import yaml

log = logging.getLogger("agent.mcp")

ALLOWLIST = pathlib.Path("agentarch/project/mcp/allowlist.yaml")


class AllowlistError(RuntimeError):
    """The allowlist and the servers disagree. Fatal at startup, never a 500 at request time."""


class RugPull(RuntimeError):
    """A server changed a tool description since it was reviewed.

    Its own exception type because it is the one failure here that is not a misconfiguration: the
    artifact you approved and the artifact serving traffic have diverged, and the right response is a
    human reading the new description rather than a retry.
    """


def load_allowlist() -> dict:
    if not ALLOWLIST.exists():
        raise AllowlistError(f"{ALLOWLIST} not found — run from the project root")
    return yaml.safe_load(ALLOWLIST.read_text())


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
                raise AllowlistError(f"{self.server['name']} closed the connection")
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
                log.info("refused %s/%s: not in tools_allow", s["name"], name)
                continue

            recorded = digests.get(name)
            if recorded and description_digest(tool.get("description", "")) != recorded:
                raise RugPull(
                    f" {s['name']}/{name} changed its description since it was reviewed.\n"
                    f"  approved {recorded[:12]}…\n"
                    f"  serving  {description_digest(tool.get('description', ''))[:12]}…\n\n"
                    "Read the new description before accepting it. Then run\n"
                    "`agentarch mcp audit --probe --record` to re-record the digest."
                )

            log.info("allowed %s/%s", s["name"], name)
            out.append({"name": f"{s['name']}__{name}",
                        "description": tool.get("description", ""),
                        "input_schema": tool.get("inputSchema", {"type": "object"})})
        return out

    def call(self, name: str, args: dict) -> dict:
        bare = name.split("__", 1)[1]
        if bare not in set(self.server.get("tools_allow", [])):
            return {"error": f"{bare} is not allowlisted", "retryable": False}
        return self._rpc("tools/call", {"name": bare, "arguments": args})
