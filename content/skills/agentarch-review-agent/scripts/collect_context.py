"""Assemble everything a review needs in one pass.

Twelve separate file reads produce twelve separate partial pictures, and the reviewer forms an
opinion before seeing the tool that contradicts it.

Usage:  python3 collect_context.py <agent-id> [--root .]
"""
from __future__ import annotations

import argparse
import pathlib
import re
import shutil

# Reviewed use: resolved CLI, fixed argv, no shell, bounded execution below.
import subprocess  # nosec B404
import sys


def section(title: str) -> None:
    print(f"\n{'=' * 78}\n{title}\n{'=' * 78}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("agent_id")
    parser.add_argument("--root", type=pathlib.Path, default=pathlib.Path("."))
    parser.add_argument("--agentarch-bin", type=pathlib.Path,
                        help="Explicit installed CLI executable; otherwise resolve agentarch on PATH")
    args = parser.parse_args()
    agent_id = args.agent_id
    if not re.fullmatch(r"[a-z][a-z0-9_-]*", agent_id):
        parser.error("agent-id must be a slug, not a filesystem path or command")
    root = args.root.resolve()

    d = root / "agentarch/project/agents" / agent_id
    if not d.exists():
        print(f"no agent {agent_id!r} at {d}", file=sys.stderr)
        available = (root / "agentarch/project/agents")
        if available.exists():
            print("available:", ", ".join(p.name for p in available.iterdir() if p.is_dir()),
                  file=sys.stderr)
        return 1

    section(f"MANIFEST — {d / 'agent.yaml'}")
    print((d / "agent.yaml").read_text())

    for prompt in sorted((d / "prompts").glob("*.md")) if (d / "prompts").exists() else []:
        section(f"PROMPT — {prompt}")
        print(prompt.read_text())

    tools = root / "agentarch/project/tools"
    for spec in sorted(tools.glob("*.tool.yaml")) if tools.exists() else []:
        section(f"TOOL — {spec.name}")
        print(spec.read_text())

    allowlist = root / "agentarch/project/mcp/allowlist.yaml"
    if allowlist.exists():
        section("MCP ALLOWLIST")
        print(allowlist.read_text())

    results = d / "evals/results"
    if results.exists():
        latest = sorted(results.iterdir())[-1:]
        for r in latest:
            section(f"LATEST EVAL RESULT — {r.name}")
            print(r.read_text())

    tm = d / "threat-model.md"
    section("THREAT MODEL")
    print(tm.read_text() if tm.exists() else "(none — this is itself a finding)")

    section("GATE OUTPUT")
    selected = str(args.agentarch_bin) if args.agentarch_bin else shutil.which("agentarch")
    try:
        if not selected:
            raise FileNotFoundError("agentarch")
        executable = str(pathlib.Path(selected).resolve(strict=True))
        # Absolute operator-selected CLI; repository paths remain argv data.
        out = subprocess.run(  # nosec B603
            [executable, "check", "--profile", "standard", "--format", "json",
             "--agent", agent_id, "--root", str(root)],
            check=False, shell=False, capture_output=True, text=True, timeout=60)
        print(f"gate exit status: {out.returncode}")
        print(out.stdout or out.stderr)
    except FileNotFoundError:
        print("(agentarch is not on PATH — run the gate yourself before reviewing)")
    except subprocess.TimeoutExpired:
        print("(gate timed out)")

    section("REMINDER")
    print("""The tools above already found what tools can find. Spend your effort on what they
cannot see:

  · does the manifest describe the agent that actually exists?
  · does out_of_scope appear in the prompt?
  · where does identity come from in each tool signature?
  · does the action guardrail consult the model?
  · were the eval thresholds written before or after the scores?""")
    return 0


if __name__ == "__main__":
    sys.exit(main())
