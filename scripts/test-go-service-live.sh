#!/usr/bin/env bash
# Real container test of the go-service profile, security floor included. No model, no network
# during verification. A small Go module is verified clean, then with a formatting error and a
# failing test, each of which must block.
# Usage: IMAGE=<go-service verifier>@sha256:<digest> AGENTARCH_BIN=/path/to/agentarch \
#        bash scripts/test-go-service-live.sh
set -euo pipefail
: "${IMAGE:?Provide a go-service verifier IMAGE pinned by @sha256 digest}"
: "${AGENTARCH_BIN:?Provide the built agentarch binary path}"
AGENTARCH_BIN="$(realpath "$AGENTARCH_BIN")"
root="$(mktemp -d /tmp/agentarch-go-live.XXXXXX)"
trap 'rm -rf "$root"' EXIT

fixture() { # dir
  mkdir -p "$1"
  git init -q "$1"
  printf 'module example.com/fixture\n\ngo 1.22\n' > "$1/go.mod"
  : > "$1/go.sum"
  cat > "$1/sum.go" <<'GO'
package fixture

// Sum adds the numbers it is given.
func Sum(values ...int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}
GO
  cat > "$1/sum_test.go" <<'GO'
package fixture

import "testing"

func TestSum(t *testing.T) {
	if got := Sum(1, 2, 3); got != 6 {
		t.Fatalf("Sum = %d, want 6", got)
	}
}
GO
}

commit() { git -C "$1" add --all && git -C "$1" -c user.name=fixture -c user.email=fixture@example.invalid commit -qm "$2"; }

run_case() { # name, dir, comma-separated check ids expected to fail ("" for none)
  commit "$2" "$1"
  set +e
  "$AGENTARCH_BIN" verify-profile --root "$2" --profile go-service --image "$IMAGE" --format json \
    --diagnostics-reason "go-service live test on a synthetic fixture" > "$2.json"
  set -e
  python3 - "$2.json" "$1" "$3" <<'PY'
import json, sys
path, name, want = sys.argv[1:]
v = json.load(open(path))
want = set(filter(None, want.split(",")))
checks = {c["id"]: c for c in v["checks"]}
failed = {k for k, c in checks.items() if c["status"] != "passed"}
for required in ("format", "vet", "build", "test", "secrets", "dependency-audit"):
    if required not in checks:
        print(f"FAIL {name}: check {required} did not run"); sys.exit(1)
if failed != want or v["passed"] != (not want):
    print(f"FAIL {name}: not passed = {sorted(failed)}, want {sorted(want)}")
    for k in sorted(failed | want):
        c = checks.get(k, {})
        print(f"  {k}: {c.get('status')} exit {c.get('exit_code')} {c.get('message', '')}")
        for line in (c.get("output_excerpt") or "").splitlines()[-15:]:
            print(f"    | {line}")
    sys.exit(1)
print(f"PASS {name}: not passed = {sorted(failed) or 'none'}")
PY
}

fixture "$root/clean"
run_case clean "$root/clean" ""

fixture "$root/unformatted"
printf 'package fixture\nfunc  Twice(v int) int { return v*2 }\n' > "$root/unformatted/twice.go"
run_case unformatted "$root/unformatted" "format"

fixture "$root/failing"
sed -i 's/got != 6/got != 7/' "$root/failing/sum_test.go"
run_case failing-test "$root/failing" "test"

echo "go-service live test passed."
