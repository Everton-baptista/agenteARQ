#!/usr/bin/env python3
"""Extract an agent's attack surface from its declared artifacts.

Compiling this by hand is how the tool nobody remembered stays out of the threat model.

Usage:  python3 inventory.py <agent-id> [--root .]
"""
from __future__ import annotations

import pathlib
import sys

try:
    import yaml
except ImportError:
    print("needs pyyaml: pip install pyyaml", file=sys.stderr)
    raise SystemExit(1)


def main() -> int:
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    if not args:
        print("usage: inventory.py <agent-id> [--root .]", file=sys.stderr)
        return 1
    root = pathlib.Path(sys.argv[sys.argv.index("--root") + 1]) if "--root" in sys.argv else pathlib.Path(".")
    d = root / "agentarch/project/agents" / args[0]

    manifest = yaml.safe_load((d / "agent.yaml").read_text())["agent"]

    print(f"\nATTACK SURFACE — {manifest['id']}")
    print(f"stage {manifest.get('stage')} · autonomy {manifest.get('autonomy', {}).get('level')}")

    print("\n── untrusted inputs ─────────────────────────────────────────")
    print("  user message                          never trusted")
    for v in manifest.get("prompts", {}).get("variables", []):
        mark = "trusted" if v.get("trusted") else "NOT TRUSTED"
        print(f"  variable {v['name']:28} {mark}  (from {v.get('source')})")
    rag = manifest.get("context", {}).get("rag", {})
    if rag.get("enabled"):
        print(f"  retrieved passages                    NOT TRUSTED  "
              f"(corpus {rag.get('corpus_id')}@{rag.get('corpus_version')})")
        print("    → anyone who can edit that corpus can write into the prompt")
    mem = manifest.get("context", {}).get("memory", {})
    if mem.get("kind") and mem["kind"] != "none":
        print(f"  recalled memory ({mem['kind']})              NOT TRUSTED")
    if manifest.get("mcp", {}).get("servers_used"):
        print("  MCP tool descriptions                 NOT TRUSTED")
    if manifest.get("handoff", {}).get("accepts_from"):
        print(f"  handoff payloads from {manifest['handoff']['accepts_from']}   NOT TRUSTED")

    print("\n── capabilities ─────────────────────────────────────────────")
    egress, secrets = set(), set()
    for entry in manifest.get("tools", []):
        spec_path = d / entry["ref"]
        if not spec_path.exists():
            print(f"  {entry['ref']}  ← BROKEN REFERENCE, and therefore unreviewed")
            continue
        t = yaml.safe_load(spec_path.read_text())["tool"]
        perms = t.get("permissions", {})
        approval = entry.get("approval", "none")
        flag = ""
        if t["effect"] in ("irreversible", "money", "communication") and approval == "none":
            flag = "  ← IRREVERSIBLE WITH NO APPROVAL"
        print(f"  {t['id']:26} {t['effect']:14} approval={approval}{flag}")
        if perms.get("data_access"):
            print(f"    reaches: {', '.join(perms['data_access'])}")
        if perms.get("sandbox"):
            print(f"    sandbox: {perms['sandbox']}")
        egress.update(perms.get("network", {}).get("egress", []))
        secrets.update(perms.get("secrets", []))

    print("\n── egress ───────────────────────────────────────────────────")
    print("  " + ("\n  ".join(sorted(egress)) if egress else "(none declared)"))
    print("  Remember the channels that do not look like network access:")
    print("  image URLs, links, DNS, query parameters on an allowed host.")

    print("\n── secrets in scope (by name) ───────────────────────────────")
    print("  " + ("\n  ".join(sorted(secrets)) if secrets else "(none)"))

    print("\n── guardrails ───────────────────────────────────────────────")
    for point in ("input", "output", "action"):
        entries = manifest.get("guardrails", {}).get(point)
        if entries is None:
            print(f"  {point:8} MISSING KEY — an oversight, not a decision")
        elif not entries:
            print(f"  {point:8} empty (recorded decision)")
        else:
            for g in entries:
                print(f"  {point:8} {g['control']}  {g['fail_mode']}")

    print("\nNow walk references/attack-trees.md against this surface.\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
