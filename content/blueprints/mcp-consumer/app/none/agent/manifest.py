"""Loading the contract, and refusing to start when the code and the contract disagree.

Everything the loop is allowed to do — which model, how many steps, which tools, what the prompt
says — is read from the manifest at startup rather than written here. That is the whole reason the
manifest is called the contract: a limit that exists in two places drifts, and the copy in the
code is the one nobody reviews.
"""

from __future__ import annotations

import hashlib
import pathlib

import yaml

# Relative to the process working directory, which for this service is the project root. The
# agent core reads its own contract; nothing about that depends on how the caller arrived.
AGENT_DIR = pathlib.Path("agentarch/project/agents/docs-assistant")


class ContractError(RuntimeError):
    """The manifest and the files it points at disagree. Always fatal at startup."""


def load_manifest() -> dict:
    """Load the manifest and verify the system prompt still hashes to what it records.

    Failing closed here is the point. A prompt edited without a version bump is an invisible
    behaviour change that silently invalidates every eval taken before it, and starting anyway
    means serving something nobody reviewed. In a service this matters more than in a script:
    the process starts once and then answers thousands of requests under the wrong assumption.
    """
    manifest = yaml.safe_load((AGENT_DIR / "agent.yaml").read_text())["agent"]
    spec = manifest["prompts"]["system"]

    raw = (AGENT_DIR / spec["path"]).read_bytes()
    actual = hashlib.sha256(raw).hexdigest()
    if actual != spec["sha256"]:
        raise ContractError(
            f"system prompt has changed but version {spec['version']} was not bumped.\n"
            f"  manifest records {spec['sha256'][:12]}…\n"
            f"  file hashes to   {actual[:12]}…\n"
            "Run `agentarch validate` — this is AA-REF-004."
        )

    manifest["_system_prompt"] = raw.decode()
    return manifest


def load_tools(manifest: dict) -> dict[str, dict]:
    """Tool specs are the source for the model-facing schema.

    The reviewed artifact and what the model actually sees cannot drift apart, because there is
    only one of them.
    """
    tools: dict[str, dict] = {}
    for entry in manifest.get("tools", []):
        spec = yaml.safe_load((AGENT_DIR / entry["ref"]).read_text())["tool"]
        spec["_approval"] = entry.get("approval", "none")
        tools[spec["id"]] = spec
    return tools
