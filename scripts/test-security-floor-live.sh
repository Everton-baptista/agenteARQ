#!/usr/bin/env bash
# Real container test of the security floor. Does not call a model or contact any service:
# verification runs with --network=none against the OSV snapshot baked into the image.
# Usage: IMAGE=<verifier with the floor>@sha256:<digest> AGENTARCH_BIN=/path/to/agentarch \
#        bash scripts/test-security-floor-live.sh
set -euo pipefail
: "${IMAGE:?Provide a verifier IMAGE that includes verifiers/floor, pinned by @sha256 digest}"
: "${AGENTARCH_BIN:?Provide the built agentarch binary path}"
AGENTARCH_BIN="$(realpath "$AGENTARCH_BIN")"
root="$(mktemp -d /tmp/agentarch-floor-live.XXXXXX)"
trap 'rm -rf "$root"' EXIT

# Assembled at runtime so this repository never contains a credential-shaped string itself.
# Random characters keep it above the scanners' entropy filters; it is not a real credential.
fake_token="ghp_$(head -c 512 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9' | head -c 36)"

new_product() { # dir
  local dir=$1
  mkdir -p "$dir"
  git init -q "$dir"
  "$AGENTARCH_BIN" workflow init --root "$dir" --name 'Floor fixture' --profile universal \
    --author floor-author --goal 'Exercise the security floor' --format json > /dev/null
  cat > "$dir/agentarch/project/product.yaml" <<'YAML'
schema_version: '1.0'
name: Floor fixture
goal: Exercise the security floor
profile: universal
author: floor-author
has_ai: false
executor_ids: []
application_agent_ids: []
checks:
  - id: smoke
    program: /bin/true
    args: []
    timeout_seconds: 20
YAML
  for file in brief.md spec.md adr/0001-architecture.md; do
    printf '# Floor acceptance\n\nAC-1: The security floor runs and reports a decisive result.\nDecision: rely on the image floor.\n' \
      > "$dir/agentarch/project/$file"
  done
  cat > "$dir/agentarch/project/tasks.yaml" <<'YAML'
schema_version: '1.0'
tasks:
  - id: floor
    title: Run the security floor
    acceptance: [AC-1]
    checks: [smoke]
    depends_on: []
YAML
  printf '# Floor fixture\n' > "$dir/README.md"
}

verify_case() { # name, dir, expected secrets status, expected dependency-audit status
  local name=$1 dir=$2
  for phase in specified planned implementing; do
    "$AGENTARCH_BIN" workflow checkpoint --root "$dir" --phase "$phase" --format json > /dev/null
  done
  "$AGENTARCH_BIN" workflow checkpoint --root "$dir" --phase implementing --task floor --format json > /dev/null
  git -C "$dir" add --all
  git -C "$dir" -c user.name='Floor fixture' -c user.email='fixture@example.invalid' commit -qm "$name"
  set +e
  "$AGENTARCH_BIN" workflow verify --root "$dir" --image "$IMAGE" --format json > "$dir.json"
  set -e
  python3 - "$dir.json" "$name" "$3" "$4" <<'PY'
import json, sys
path, name, want_secrets, want_audit = sys.argv[1:]
v = json.load(open(path))
checks = {c["id"]: c for c in v["state"]["verification"]["checks"]}
got = {k: checks[k]["status"] for k in ("secrets", "dependency-audit")}
want = {"secrets": want_secrets, "dependency-audit": want_audit}
if got != want:
    print(f"FAIL {name}: got {got}, want {want}")
    for k in got:
        print(f"  {k}: exit {checks[k]['exit_code']} {checks[k].get('message', '')}")
    sys.exit(1)
if v["state"]["verification"]["passed"] != (want_secrets == want_audit == "passed"):
    print(f"FAIL {name}: passed={v['state']['verification']['passed']}")
    sys.exit(1)
print(f"PASS {name}: secrets={got['secrets']} dependency-audit={got['dependency-audit']}")
PY
}

# 1. Nothing to find: no secret, no dependency manifest.
new_product "$root/clean"
verify_case clean "$root/clean" passed passed

# 2. A committed credential fails the secrets check.
new_product "$root/secret"
printf 'GITHUB_TOKEN = "%s"\n' "$fake_token" > "$root/secret/settings.py"
verify_case committed-secret "$root/secret" failed passed

# 3. The project cannot switch the scanner off with its own config or ignore file.
new_product "$root/secret-disabled"
printf 'GITHUB_TOKEN = "%s"\n' "$fake_token" > "$root/secret-disabled/settings.py"
printf '[allowlist]\npaths = [".*"]\n' > "$root/secret-disabled/.gitleaks.toml"
printf 'settings.py:github-pat:1\n' > "$root/secret-disabled/.gitleaksignore"
verify_case secret-with-project-overrides "$root/secret-disabled" failed passed

# 4. A pinned dependency with published advisories fails the audit, offline.
new_product "$root/vulnerable"
printf 'django==3.2.0\n' > "$root/vulnerable/requirements.txt"
verify_case vulnerable-dependency "$root/vulnerable" passed failed

# 5. ...and an osv-scanner.toml in the project cannot ignore it.
new_product "$root/vulnerable-ignored"
printf 'django==3.2.0\n' > "$root/vulnerable-ignored/requirements.txt"
cat > "$root/vulnerable-ignored/osv-scanner.toml" <<'TOML'
[[PackageOverrides]]
name = "django"
ecosystem = "PyPI"
ignore = true
reason = "attempt to hide a vulnerable dependency"
TOML
verify_case vulnerable-with-project-ignore "$root/vulnerable-ignored" passed failed

# 6. A manifest the scanner cannot audit is inconclusive, never a pass.
new_product "$root/no-lockfile"
printf '{"name":"fixture","version":"1.0.0","dependencies":{"left-pad":"^1.3.0"}}\n' > "$root/no-lockfile/package.json"
verify_case manifest-without-lockfile "$root/no-lockfile" passed inconclusive

echo "Security floor live test passed."
