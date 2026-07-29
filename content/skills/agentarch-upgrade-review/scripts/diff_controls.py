#!/usr/bin/env python3
"""Compare the installed control catalogue with the one a newer CLI carries.

Usage:  python3 diff_controls.py [--root .]

Run it before `agentarch upgrade`, so the surprises arrive as a list rather than as a red build.
"""
from __future__ import annotations

import json
import pathlib
import subprocess
import sys

try:
    import yaml
except ImportError:
    print("needs pyyaml: pip install pyyaml", file=sys.stderr)
    raise SystemExit(1)


def installed_packs(root: pathlib.Path) -> dict:
    """Read the packs as they stand in this project."""
    out = {}
    base = root / "agentarch/std/packs"
    for p in base.glob("*/pack.yaml"):
        doc = yaml.safe_load(p.read_text())["pack"]
        for r in doc.get("requires", []):
            out[(doc["id"], r["control"])] = {
                "severity": r["severity"],
                "enforced_from": r.get("enforced_from"),
                "pack_version": doc["version"],
            }
    return out


def main() -> int:
    root = pathlib.Path(sys.argv[sys.argv.index("--root") + 1]) if "--root" in sys.argv else pathlib.Path(".")

    before = installed_packs(root)
    if not before:
        print("no installed packs found — run `agentarch init` first", file=sys.stderr)
        return 1

    # The new catalogue comes from the binary on PATH, which is the one that would upgrade.
    tmp = pathlib.Path(".agentarch-upgrade-preview")
    try:
        subprocess.run(["agentarch", "init", "--root", str(tmp)],
                       capture_output=True, check=True, timeout=60)
    except FileNotFoundError:
        print("agentarch is not on PATH", file=sys.stderr)
        return 1
    except subprocess.CalledProcessError as e:
        print(f"could not stage the new content: {e.stderr.decode()[:300]}", file=sys.stderr)
        return 1

    after = installed_packs(tmp)

    added, raised, now_enforced, removed = [], [], [], []
    rank = {"minor": 1, "major": 2, "blocker": 3}

    for key, new in after.items():
        old = before.get(key)
        if old is None:
            added.append((key, new))
            continue
        if rank.get(new["severity"], 0) > rank.get(old["severity"], 0):
            raised.append((key, old, new))
        # A grace period ending is not a regression; it is the design working.
        if old.get("enforced_from") and not new.get("enforced_from"):
            now_enforced.append((key, new))
    for key, old in before.items():
        if key not in after:
            removed.append((key, old))

    def show(title, rows, fmt):
        print(f"\n{title}")
        if not rows:
            print("  (none)")
        for r in rows:
            print("  " + fmt(r))

    show("CONTROLS ADDED — these enter in warn mode unless the pack says otherwise",
         added, lambda r: f"{r[0][1]}  ({r[0][0]}, {r[1]['severity']})")
    show("SEVERITY RAISED — these will start blocking",
         raised, lambda r: f"{r[0][1]}  {r[1]['severity']} → {r[2]['severity']}  ({r[0][0]})")
    show("GRACE PERIOD ENDED — enforced from now on, as designed",
         now_enforced, lambda r: f"{r[0][1]}  ({r[0][0]}, {r[1]['severity']})")
    show("CONTROLS REMOVED",
         removed, lambda r: f"{r[0][1]}  ({r[0][0]})")

    import shutil
    shutil.rmtree(tmp, ignore_errors=True)

    total = len(added) + len(raised) + len(now_enforced) + len(removed)
    print(f"\n{total} change(s). Run `agentarch check` after upgrading; new failures are\n"
          "expected, and `agentarch explain <control.id>` says what each one wants.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
