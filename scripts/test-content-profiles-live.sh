#!/usr/bin/env bash
# Real container test of the static-site or docs profile, security floor included. No model, no
# network during verification. Each fixture under verifiers/<profile>/fixtures/ is committed to a
# throwaway Git repository and verified with `agentarch verify-profile`: `pass` must pass, and
# each `fail-*` fixture must block on exactly the checks listed below, including fixtures that
# try to switch rules off with their own config files and inline comments.
# Usage: PROFILE=static-site|docs IMAGE=<verifier>@sha256:<digest> AGENTARCH_BIN=/path/to/agentarch \
#        bash scripts/test-content-profiles-live.sh
set -euo pipefail
: "${PROFILE:?Set PROFILE to static-site or docs}"
: "${IMAGE:?Provide the PROFILE verifier IMAGE pinned by @sha256 digest}"
: "${AGENTARCH_BIN:?Provide the built agentarch binary path}"
AGENTARCH_BIN="$(realpath "$AGENTARCH_BIN")"
here="$(cd "$(dirname "$0")/.." && pwd)"
fixtures="$here/verifiers/$PROFILE/fixtures"
root="$(mktemp -d /tmp/agentarch-content-live.XXXXXX)"
trap 'rm -rf "$root"' EXIT

# case name, checks expected not to pass, check=substring pairs expected in the output excerpts
case "$PROFILE" in
  static-site)
    checks="html,lint,links"
    cases=(
      "pass||html=3 HTML files valid;lint=(2 as modules) clean;links=all 15 relative references"
      "fail-html|html|html=inline html-validate directive(s) in the project were not applied;html=close-order;html=wcag/h37"
      "fail-lint|lint|lint=inline eslint rule directive(s) in the project were not applied;lint=no-undef;lint=no-unused-vars"
      "fail-links|links|links=contact.html does not exist;links=docs/index.html does not exist;links=points outside the project"
      "fail-no-html|html,links|html=no .html files found;links=no .html files found;lint=no JavaScript files"
    ) ;;
  docs)
    checks="markdown,links"
    cases=(
      "pass||markdown=3 Markdown files clean;links=all 9 relative references"
      "fail-markdown|markdown|markdown=MD001/heading-increment;markdown=MD025;markdown=MD042"
      "fail-links|links|links=docs/missing.md does not exist;links=img/missing.svg does not exist;links=docs/also-missing.md does not exist"
      "fail-no-markdown|markdown,links|markdown=no Markdown (.md) files found;links=no Markdown (.md) files found"
    ) ;;
  *) echo "PROFILE must be static-site or docs" >&2; exit 2 ;;
esac

for entry in "${cases[@]}"; do
  IFS='|' read -r name want expect <<< "$entry"
  dir="$root/$name"
  mkdir -p "$dir"
  cp -R "$fixtures/$name/." "$dir/"
  git init -q "$dir"
  git -C "$dir" add --all
  git -C "$dir" -c user.name=fixture -c user.email=fixture@example.invalid commit -qm "$name"
  set +e
  "$AGENTARCH_BIN" verify-profile --root "$dir" --profile "$PROFILE" --image "$IMAGE" --format json \
    --diagnostics-reason "$PROFILE live test on a synthetic fixture" > "$dir.json"
  set -e
  python3 - "$dir.json" "$name" "$checks" "$want" "$expect" <<'PY'
import json, sys
path, name, checks, want, expect = sys.argv[1:]
v = json.load(open(path))
want = set(filter(None, want.split(",")))
got = {c["id"]: c for c in v["checks"]}
failed = {k for k, c in got.items() if c["status"] != "passed"}
problems = []
for required in checks.split(",") + ["secrets", "dependency-audit"]:
    if required not in got:
        problems.append(f"check {required} did not run")
if failed != want or v["passed"] != (not want):
    problems.append(f"not passed = {sorted(failed)}, want {sorted(want)}")
for pair in filter(None, expect.split(";")):
    check, text = pair.split("=", 1)
    if text not in (got.get(check, {}).get("output_excerpt") or ""):
        problems.append(f"{check} output lacks {text!r}")
for k in sorted(got):
    c = got[k]
    print(f"  {name} {k}: {c['status']} exit {c.get('exit_code')}")
    for line in (c.get("output_excerpt") or "").splitlines()[-12:]:
        print(f"    | {line}")
if problems:
    for p in problems:
        print(f"FAIL {name}: {p}")
    sys.exit(1)
print(f"PASS {name}: not passed = {sorted(failed) or 'none'}")
PY
done

echo "$PROFILE live test passed."
