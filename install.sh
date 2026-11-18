#!/bin/sh
# Install an authenticated agentarch release. Requires curl, tar and cosign.
# AGENTARCH_VERSION: exact vX.Y.Z release tag (default: latest release).
# AGENTARCH_BIN_DIR: destination (default: /usr/local/bin or ~/.local/bin).
set -eu

REPO="Everton-baptista/agenteARQ"
die() { printf '\nerror: %s\n' "$1" >&2; exit 1; }
say() { printf '%s\n' "$1"; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required and was not found"; }

case "${1:-}" in
  --help|-h)
    say "Install an authenticated agentarch release (curl, tar, cosign required)."
    say "Usage: AGENTARCH_VERSION=vX.Y.Z AGENTARCH_BIN_DIR=path sh install.sh"
    say "The release workflow identity and SHA-256 checksum are mandatory checks."
    exit 0 ;;
  '') ;;
  *) die "unsupported argument: $1 (use --help)" ;;
esac
[ "$#" -le 1 ] || die "unexpected arguments (use --help)"

os=$(uname -s)
arch=$(uname -m)
case "$os" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "unsupported operating system: $os; see https://github.com/$REPO/releases" ;;
esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac
need curl
need tar
command -v cosign >/dev/null 2>&1 || die "cosign is required to authenticate the release. Install it from https://docs.sigstore.dev/cosign/system_config/installation/ and retry. No unsigned installation is supported."
if command -v sha256sum >/dev/null 2>&1; then
  hash_program=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  hash_program=shasum
else
  die "sha256sum or shasum is required to verify the archive"
fi

tmp=$(mktemp -d)
staged=''
cleanup() {
  rm -rf "$tmp"
  if [ -n "$staged" ]; then rm -f "$staged"; fi
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM
download() {
  curl --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 --max-time 180 -fsSL -o "$2" "$1"
}

version=${AGENTARCH_VERSION:-}
if [ -z "$version" ]; then
  say "resolving the latest release…"
  download "https://api.github.com/repos/$REPO/releases/latest" "$tmp/latest.json" || die "could not resolve latest release; set AGENTARCH_VERSION=vX.Y.Z"
  version=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp/latest.json")
fi
# Restrict filenames, URLs and certificate subjects to a single release tag.
printf '%s\n' "$version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?(\+[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$' || die "release tag must be vX.Y.Z (optionally with a prerelease/build suffix)"
case "$version" in *'
'*) die "release tag must be a single line" ;; esac
plain=${version#v}
archive="agentarch_${plain}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"
identity="https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$version"

say "downloading signed checksums ($version)…"
for asset in checksums.txt checksums.txt.sigstore.json; do
  download "$base/$asset" "$tmp/$asset" || die "could not download $asset; the release must publish signed checksums"
done

# Authenticate the checksum document before trusting filenames or downloading code.
# Exact subject prevents a signature from another workflow, branch or release tag.
cosign verify-blob "$tmp/checksums.txt" \
  --bundle "$tmp/checksums.txt.sigstore.json" \
  --certificate-identity "$identity" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ||
  die "release signature verification failed; nothing was installed"
want=$(awk -v name="$archive" '$2 == name { count++; digest=$1 } END { if (count != 1 || length(digest) != 64 || digest ~ /[^0-9a-f]/) exit 1; print digest }' "$tmp/checksums.txt") ||
  die "$archive must appear exactly once with a SHA-256 digest in signed checksums.txt"

say "downloading $archive…"
download "$base/$archive" "$tmp/$archive" || die "could not download $archive"
if [ "$hash_program" = sha256sum ]; then
  got=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
else
  got=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
fi
[ "$got" = "$want" ] || die "checksum mismatch for $archive; nothing was installed"
say "signature identity and archive checksum verified"

tar -xzf "$tmp/$archive" -C "$tmp" agentarch || die "could not extract agentarch from $archive"
[ -f "$tmp/agentarch" ] && [ ! -L "$tmp/agentarch" ] || die "archive must contain a regular agentarch binary"
bindir=${AGENTARCH_BIN_DIR:-}
if [ -z "$bindir" ]; then
  if [ -w /usr/local/bin ]; then bindir=/usr/local/bin; else bindir="$HOME/.local/bin"; fi
fi
case "$bindir" in /*) ;; *) bindir="./$bindir" ;; esac
[ ! -d "$bindir/agentarch" ] || die "destination agentarch is a directory"
mkdir -p "$bindir" || die "cannot create $bindir"
# Stage on the destination filesystem so replacement is a single rename.
staged=$(mktemp "$bindir/.agentarch-install.XXXXXX") || die "cannot stage installation in $bindir"
cp "$tmp/agentarch" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$bindir/agentarch"
staged=''
say "installed agentarch $version to $bindir/agentarch"
case ":$PATH:" in
  *":$bindir:"*) ;;
  *) say "Add $bindir to your PATH to use agentarch from any directory." ;;
esac
say "Next, run agentarch start in your project directory."
