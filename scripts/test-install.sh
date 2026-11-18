#!/bin/sh
# Hermetic installer contract tests: no network, signature service, or binary execution.
set -eu
project=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' 0
trap 'exit 130' INT
trap 'exit 143' TERM
mockbin="$test_root/bin"
fixture="$test_root/release"
mkdir -p "$mockbin" "$fixture" "$test_root/source"
for program in uname tar gzip sha256sum mktemp rm grep sed cut awk mkdir cp chmod mv cat; do
  ln -s "$(command -v "$program")" "$mockbin/$program"
done
real_tar=$(command -v tar)
rm "$mockbin/tar"
cat > "$mockbin/tar" <<'EOF'
#!/bin/sh
printf '%s\n' extract >> "$MOCK_LOG"
exec "$MOCK_REAL_TAR" "$@"
EOF
cat > "$mockbin/curl" <<'EOF'
#!/bin/sh
set -eu
out=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    --proto|--tlsv1.2|--retry|--connect-timeout|--max-time)
      if [ "$1" = --tlsv1.2 ]; then shift; else shift 2; fi ;;
    -fsSL) shift ;;
    https://*) url=$1; shift ;;
    *) exit 90 ;;
  esac
done
asset=${url##*/}
if [ "$asset" = latest ]; then asset=latest.json; fi
printf '%s\n' "download:$asset" >> "$MOCK_LOG"
[ "$asset" != "${MOCK_MISSING:-}" ] || exit 22
cp "$MOCK_FIXTURE/$asset" "$out"
EOF
cat > "$mockbin/cosign" <<'EOF'
#!/bin/sh
set -eu
[ "$1" = verify-blob ] || exit 91
blob=$2
shift 2
identity=''
issuer=''
bundle=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --bundle) bundle=$2 ;;
    --certificate-identity) identity=$2 ;;
    --certificate-oidc-issuer) issuer=$2 ;;
    *) exit 92 ;;
  esac
  shift 2
done
[ "$issuer" = https://token.actions.githubusercontent.com ] || exit 93
[ "$identity" = "https://github.com/Everton-baptista/agenteARQ/.github/workflows/release.yml@refs/tags/v1.2.3" ] || exit 94
[ "$(sed -n '1p' "$bundle")" = valid-signature ] || exit 95
[ "$(sed -n '2p' "$bundle")" = "$identity" ] || exit 96
[ -s "$blob" ] || exit 97
printf '%s\n' verified >> "$MOCK_LOG"
EOF
chmod +x "$mockbin/curl" "$mockbin/cosign" "$mockbin/tar"
printf '#!/bin/sh\nprintf executed > "%s"\n' "$test_root/executed" > "$test_root/source/agentarch"
chmod +x "$test_root/source/agentarch"
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) exit 1 ;; esac
case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; *) arch=amd64 ;; esac
archive="agentarch_1.2.3_${os}_${arch}.tar.gz"
"$real_tar" -czf "$test_root/valid.tar.gz" -C "$test_root/source" agentarch
identity='https://github.com/Everton-baptista/agenteARQ/.github/workflows/release.yml@refs/tags/v1.2.3'

reset_fixture() {
  cp "$test_root/valid.tar.gz" "$fixture/$archive"
  digest=$(sha256sum "$fixture/$archive" | cut -d ' ' -f 1)
  printf '%s  %s\n' "$digest" "$archive" > "$fixture/checksums.txt"
  # The mock's two-line bundle models signature + identity; no cryptography is mocked as real.
  printf '%s\n%s\n' valid-signature "$identity" > "$fixture/checksums.txt.sigstore.json"
  printf '%s\n' '{"tag_name":"v1.2.3"}' > "$fixture/latest.json"
  mkdir -p "$test_root/install"
  printf '%s\n' previous-binary > "$test_root/install/agentarch"
  : > "$test_root/log"
  missing=''
  version=v1.2.3
}
run_installer() {
  PATH="$mockbin" MOCK_FIXTURE="$fixture" MOCK_LOG="$test_root/log" MOCK_REAL_TAR="$real_tar" \
    MOCK_MISSING="$missing" AGENTARCH_VERSION="$version" AGENTARCH_BIN_DIR="$test_root/install" \
    /bin/sh "$project/scripts/install.sh" > "$test_root/output" 2>&1
}
failed_install() {
  label=$1
  if run_installer; then cat "$test_root/output"; printf 'FAIL: %s succeeded\n' "$label"; exit 1; fi
  [ "$(cat "$test_root/install/agentarch")" = previous-binary ] || { printf 'FAIL: %s modified destination\n' "$label"; exit 1; }
  [ ! -e "$test_root/executed" ] || { printf 'FAIL: binary executed\n'; exit 1; }
  if grep -q '^extract$' "$test_root/log"; then printf 'FAIL: %s extracted before rejection\n' "$label"; exit 1; fi
  printf 'ok - %s\n' "$label"
}

reset_fixture
run_installer || { cat "$test_root/output"; exit 1; }
cmp "$test_root/source/agentarch" "$test_root/install/agentarch"
[ -x "$test_root/install/agentarch" ]
[ ! -e "$test_root/executed" ]
expected=$(printf 'download:checksums.txt\ndownload:checksums.txt.sigstore.json\nverified\ndownload:%s\nextract' "$archive")
[ "$(cat "$test_root/log")" = "$expected" ]
printf 'ok - authenticates before downloading/extracting, replaces binary without running it\n'

reset_fixture; version=''; run_installer
printf 'ok - resolves and authenticates exact latest tag\n'
reset_fixture; missing=checksums.txt.sigstore.json; failed_install 'missing Sigstore bundle'
reset_fixture; printf invalid > "$fixture/checksums.txt.sigstore.json"; failed_install 'invalid signature'
reset_fixture; printf '%s\n%s\n' valid-signature "${identity%v1.2.3}v1.2.2" > "$fixture/checksums.txt.sigstore.json"; failed_install 'signature from another release'
reset_fixture; printf '%s\n%s\n' valid-signature 'https://github.com/Everton-baptista/agenteARQ/.github/workflows/ci.yml@refs/tags/v1.2.3' > "$fixture/checksums.txt.sigstore.json"; failed_install 'signature from another workflow'
reset_fixture; printf tampered >> "$fixture/$archive"; failed_install 'tampered archive'
reset_fixture; cat "$fixture/checksums.txt" > "$test_root/checksum-copy"; cat "$test_root/checksum-copy" >> "$fixture/checksums.txt"; failed_install 'duplicate checksum entry'
reset_fixture; printf '%s\n' malformed > "$fixture/checksums.txt"; failed_install 'missing archive checksum'
reset_fixture; version='v1.2.3/../../other'; failed_install 'unsafe version'
reset_fixture; version=$(printf 'v1.2.3\nv1.2.4'); failed_install 'multiline version'
reset_fixture; mv "$mockbin/cosign" "$test_root/cosign"; failed_install 'missing cosign'; mv "$test_root/cosign" "$mockbin/cosign"
reset_fixture; mv "$mockbin/sha256sum" "$test_root/sha256sum"; failed_install 'missing checksum tool'; mv "$test_root/sha256sum" "$mockbin/sha256sum"

printf 'installer contract tests passed\n'
