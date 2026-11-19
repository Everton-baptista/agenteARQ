#!/usr/bin/env bash
# Real OCI smoke test. Does not call a model, publish a PR, or claim model-quality gains.
# Usage: IMAGE=alpine@sha256:<installed-digest> AGENTARCH_BIN=/path/to/agentarch bash scripts/test-workflow-live.sh
set -euo pipefail
: "${IMAGE:?Provide an installed verifier IMAGE pinned by @sha256 digest}"
: "${AGENTARCH_BIN:?Provide the built agentarch binary path}"
case "$IMAGE" in *@sha256:*) ;; *) echo "IMAGE must be digest-pinned" >&2; exit 2;; esac
AGENTARCH_BIN="$(realpath "$AGENTARCH_BIN")"
ADR_FIXTURE="$(cd "$(dirname "$0")" && pwd)/fixtures/adr-0001.md"
# Same preference order as the CLI (podman, then docker), so the image this script expects is the
# one the verifier will look for and the leak check below inspects the runtime that actually ran.
runtime=""
for candidate in podman docker; do
  if command -v "$candidate" >/dev/null 2>&1; then runtime="$candidate"; break; fi
done
[[ -n "$runtime" ]] || { echo "Install Podman or Docker" >&2; exit 2; }
"$runtime" image inspect "$IMAGE" >/dev/null 2>&1 || { echo "IMAGE is not installed in $runtime; run: $runtime pull $IMAGE" >&2; exit 2; }
live_root="$(mktemp -d /tmp/agentarch-workflow-live.XXXXXX)"
cleanup() { if [[ "${KEEP_TMP:-0}" == 1 ]]; then echo "Evidence retained at $live_root"; else rm -rf "$live_root"; fi; }
trap cleanup EXIT
product_root="$live_root/product"
mkdir -p "$product_root"
git init -q "$product_root"
"$AGENTARCH_BIN" workflow init --root "$product_root" --name 'OCI isolation smoke fixture' --profile universal --author smoke-author --goal 'Verify isolated execution without accessing host secrets or network' --format json > "$live_root/init.json"
cat > "$product_root/agentarch/project/product.yaml" <<'YAML'
schema_version: '1.0'
name: OCI isolation smoke fixture
goal: Verify isolated execution without accessing host secrets or network
profile: universal
author: smoke-author
has_ai: false
executor_ids: []
application_agent_ids: []
checks:
  - id: isolation
    program: /bin/sh
    args: [scripts/check.sh]
    timeout_seconds: 20
YAML
for file in brief.md spec.md; do
  cat > "$product_root/agentarch/project/$file" <<'DOC'
# Executable isolation acceptance

AC-1: No Git metadata, ignored secrets, credential environment, host socket or network is exposed.
AC-2: Source and root mounts reject writes; writes within the disposable workspace are permitted.
Decision: isolate the source copy in network-disabled OCI with bounded writable tmpfs.
DOC
done
cp "$ADR_FIXTURE" "$product_root/agentarch/project/adr/0001-architecture.md"
cat > "$product_root/agentarch/project/tasks.yaml" <<'YAML'
schema_version: '1.0'
tasks:
  - id: isolation-smoke
    title: Verify the container isolation boundaries
    acceptance: [AC-1, AC-2]
    checks: [isolation]
    depends_on: []
YAML
mkdir -p "$product_root/scripts"
cat > "$product_root/scripts/check.sh" <<'SH'
#!/bin/sh
set -eu
test ! -e .git
test ! -e .env
test ! -S /var/run/docker.sock
test -z "${AGENTARCH_LIVE_SECRET+x}"
if touch /source/host-was-modified 2>/dev/null; then exit 11; fi
if touch /outside-workspace 2>/dev/null; then exit 12; fi
awk 'NR>1 {found=1} END {exit found ? 1 : 0}' /proc/net/route
grep -q '^CapEff:[[:space:]]*0000000000000000$' /proc/self/status
printf 'temporary artifact\n' > generated.txt
test -s generated.txt
SH
printf '.env\n' > "$product_root/.gitignore"
printf 'TEST_DUMMY_SECRET=not-a-real-secret\n' > "$product_root/.env"
printf '# Live smoke fixture\n' > "$product_root/README.md"
for phase in specified planned implementing; do
  "$AGENTARCH_BIN" workflow checkpoint --root "$product_root" --phase "$phase" --format json > /dev/null
done
"$AGENTARCH_BIN" workflow checkpoint --root "$product_root" --phase implementing --task isolation-smoke --format json > /dev/null
git -C "$product_root" add --all
git -C "$product_root" -c user.name='Workflow smoke fixture' -c user.email='fixture@example.invalid' commit -qm 'Isolated workflow fixture'
export AGENTARCH_LIVE_SECRET=only-the-host-sees-this-test-value
"$AGENTARCH_BIN" workflow verify --root "$product_root" --image "$IMAGE" --format json > "$live_root/verified.json" || {
  code=$?; echo "workflow verify exited $code under $runtime:" >&2; cat "$live_root/verified.json" >&2; exit "$code"; }
"$AGENTARCH_BIN" workflow review --root "$product_root" --reviewer smoke-independent-reviewer --kind human --verdict approved --summary 'Isolation checks inspected in this infrastructure smoke fixture; no model quality claim.' --format json > "$live_root/reviewed.json"
python3 - "$live_root/reviewed.json" "$product_root" <<'PY'
import json,pathlib,sys
state=json.load(open(sys.argv[1]))['state']
assert state['phase']=='pr_ready',state
assert state['verification']['passed'] is True
assert state['verification']['isolation'].startswith('oci:')
root=pathlib.Path(sys.argv[2])
assert not (root/'generated.txt').exists()
assert not (root/'host-was-modified').exists()
print('PASS real OCI check → independent-review fixture → pr_ready; host source unchanged')
PY
printf '\nChanged after approval\n' >> "$product_root/README.md"
"$AGENTARCH_BIN" workflow status --root "$product_root" --format json > "$live_root/stale.json"
python3 - "$live_root/stale.json" <<'PY'
import json,sys
v=json.load(open(sys.argv[1]));v=v.get('snapshot') or v
assert v['state']['phase']=='verifying',v
assert any(x['id']=='evidence.stale' for x in v['findings'])
print('PASS source edit invalidates prior readiness')
PY
cat >> "$product_root/agentarch/project/product.yaml" <<'YAML'
  - id: timeout
    program: /bin/sleep
    args: ['10']
    timeout_seconds: 1
YAML
git -C "$product_root" add README.md agentarch/project/product.yaml
git -C "$product_root" -c user.name='Workflow smoke fixture' -c user.email='fixture@example.invalid' commit -qm 'Timeout acceptance fixture'
set +e
"$AGENTARCH_BIN" workflow verify --root "$product_root" --image "$IMAGE" --format json > "$live_root/timeout.json"
verify_code=$?
set -e
test "$verify_code" -eq 4
python3 - "$live_root/timeout.json" <<'PY'
import json,sys
v=json.load(open(sys.argv[1]))
c=next(x for x in v['state']['verification']['checks'] if x['id']=='timeout')
assert c['status']=='inconclusive',c
assert 'timed out' in c['message'],c
assert v['state']['verification']['passed'] is False
print('PASS timeout is inconclusive and blocks readiness')
PY
# Only inspect containers belonging to this unique fixture; never stop existing containers.
project_hash="$(printf '%s' "$product_root" | sha256sum | cut -d' ' -f1)"
if [[ -n "$("$runtime" ps -q --filter "label=agentarch.workflow.root=$project_hash")" ]]; then
  echo 'Unexpected surviving fixture container' >&2
  exit 1
fi
printf 'Real OCI smoke passed. Evidence directory: %s\n' "$live_root"
