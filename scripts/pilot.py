"""Collect and summarize the lay-user pilot (docs/pilot-protocol.md).

    python3 scripts/pilot.py collect --product DIR --participant P01 --session 1 --out pilot
    python3 scripts/pilot.py summarize --out pilot

`collect` records, for one participant and session, what agentarch itself knows: its version,
the workflow status and the exported evidence. It reads; it never changes the product.
`summarize` joins those records with each participant's facilitator log (pilot/<id>/log.csv,
from docs/pilot-kit/log-template.csv) and decides the protocol's readiness criteria.

No prompt or response text is collected: the screen recording is the only place it exists.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import pathlib
import re
import subprocess
import sys
from collections import Counter

PHASES = [
    "draft", "specified", "planned", "implementing", "verifying",
    "reviewing", "pr_ready", "released", "production_verified",
]
PARTICIPANT = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$")
LOG_KINDS = {"stop", "intervention", "unclear_refusal", "bypass", "fabricated_reviewer", "note"}
READY_SHARE = 0.8


def run(args: list[str], cwd: pathlib.Path | None = None) -> str:
    done = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=120, check=False)
    if done.returncode != 0:
        raise RuntimeError(f"{' '.join(args)} failed: {done.stderr.strip()}")
    return done.stdout


def collect(product: pathlib.Path, participant: str, session: int, out: pathlib.Path, binary: str) -> pathlib.Path:
    if not PARTICIPANT.match(participant):
        raise ValueError("participant id must be a short slug such as P01")
    if session < 1:
        raise ValueError("session starts at 1")
    status = json.loads(run([binary, "workflow", "status", "--format", "json", "--root", str(product)]))
    version = run([binary, "version", "--root", str(product)]).strip().splitlines()
    evidence_path = product / "agentarch" / "project" / "evidence" / "latest.json"
    evidence = json.loads(evidence_path.read_text()) if evidence_path.exists() else None
    record = {
        "schema_version": "1.0",
        "participant": participant,
        "session": session,
        "collected_at": dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds"),
        "agentarch_version": version,
        "status": status,
        "evidence": evidence,
    }
    target = out / participant / f"session-{session}.json"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(record, indent=2, ensure_ascii=False) + "\n")
    return target


def final_phase(record: dict) -> str:
    snapshot = (record.get("status") or {}).get("snapshot") or {}
    return ((snapshot.get("state") or {}).get("phase")) or "draft"


def read_log(path: pathlib.Path) -> list[dict]:
    if not path.exists():
        return []
    with path.open(newline="", encoding="utf-8") as f:
        rows = [r for r in csv.DictReader(f) if any((v or "").strip() for v in r.values())]
    for i, row in enumerate(rows, start=2):
        if row.get("kind") not in LOG_KINDS:
            raise ValueError(f"{path}:{i}: kind must be one of {sorted(LOG_KINDS)}")
    return rows


def summarize(out: pathlib.Path) -> dict:
    participants = []
    obstacles: Counter[str] = Counter()
    for folder in sorted(p for p in out.iterdir() if p.is_dir()):
        sessions = sorted(folder.glob("session-*.json"), key=lambda p: int(p.stem.split("-")[1]))
        if not sessions:
            continue
        last = json.loads(sessions[-1].read_text())
        log = read_log(folder / "log.csv")
        kinds = Counter(r["kind"] for r in log)
        stops = [r for r in log if r["kind"] == "stop"]
        for step in {(r.get("step") or "").strip().lower() for r in stops}:
            if step:
                obstacles[step] += 1
        phase = final_phase(last)
        participants.append({
            "id": folder.name,
            "sessions": len(sessions),
            "phase": phase,
            "reached_production": phase == "production_verified",
            "unaided": kinds["intervention"] == 0,
            "stops": len(stops),
            "unclear_refusals": kinds["unclear_refusal"],
            "integrity_breaches": kinds["bypass"] + kinds["fabricated_reviewer"],
        })
    n = len(participants)
    success = sum(1 for p in participants if p["reached_production"] and p["unaided"])
    repeated = sorted(step for step, count in obstacles.items() if count > 1)
    criteria = {
        "share_unaided_to_production": round(success / n, 3) if n else 0.0,
        "unaided_share_met": n > 0 and success / n >= READY_SHARE,
        "no_integrity_breach": all(p["integrity_breaches"] == 0 for p in participants),
        "repeated_obstacles": repeated,
    }
    criteria["ready_on_measured_criteria"] = (
        criteria["unaided_share_met"] and criteria["no_integrity_breach"] and not repeated
    )
    return {"participants": participants, "criteria": criteria}


def render(summary: dict) -> str:
    lines = [
        "| Participant | Sessions | Phase reached | Unaided | Stops | Unclear refusals | Integrity breaches |",
        "|---|---|---|---|---|---|---|",
    ]
    for p in summary["participants"]:
        lines.append(
            f"| {p['id']} | {p['sessions']} | {p['phase']} | {'yes' if p['unaided'] else 'no'} | "
            f"{p['stops']} | {p['unclear_refusals']} | {p['integrity_breaches']} |"
        )
    c = summary["criteria"]
    lines += [
        "",
        f"- Reached production_verified unaided: {c['share_unaided_to_production']:.0%} (needs {READY_SHARE:.0%})",
        f"- No reviewer fabricated, generated file edited or gate bypassed: {'yes' if c['no_integrity_breach'] else 'no'}",
        f"- Obstacles that stopped more than one participant: {', '.join(c['repeated_obstacles']) or 'none'}",
        f"- Ready on these criteria: {'yes' if c['ready_on_measured_criteria'] else 'no'}",
        "",
        "Hidden-test defects, the baseline comparison and the design panel are judged separately (docs/pilot-protocol.md).",
    ]
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    sub = parser.add_subparsers(dest="command", required=True)
    c = sub.add_parser("collect")
    c.add_argument("--product", required=True, type=pathlib.Path)
    c.add_argument("--participant", required=True)
    c.add_argument("--session", required=True, type=int)
    c.add_argument("--out", default="pilot", type=pathlib.Path)
    c.add_argument("--agentarch", default="agentarch")
    s = sub.add_parser("summarize")
    s.add_argument("--out", default="pilot", type=pathlib.Path)
    s.add_argument("--format", choices=["markdown", "json"], default="markdown")
    a = parser.parse_args(argv)
    try:
        if a.command == "collect":
            print(collect(a.product, a.participant, a.session, a.out, a.agentarch))
            return 0
        summary = summarize(a.out)
        print(json.dumps(summary, indent=2) if a.format == "json" else render(summary))
        return 0 if summary["criteria"]["ready_on_measured_criteria"] else 1
    except (RuntimeError, ValueError, OSError) as e:
        print(f"pilot: {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
