#!/bin/sh
# Install an authenticated agentarch release. Requires curl, tar and sha256sum (or shasum).
# Uses cosign from PATH when present; otherwise fetches a SHA-256-pinned Sigstore build for this run.
# AGENTARCH_VERSION: exact vX.Y.Z release tag (default: latest release).
# AGENTARCH_BIN_DIR: destination (default: /usr/local/bin or ~/.local/bin).
set -eu

REPO="Everton-baptista/agenteARQ"
die() { printf '\nerror: %s\n' "$1" >&2; exit 1; }
say() { printf '%s\n' "$1"; }
need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required and was not found"; }

case "${1:-}" in
  --help|-h)
    say "Install an authenticated agentarch release (needs curl, tar and sha256sum; cosign is fetched if missing)."
    say "Usage: AGENTARCH_VERSION=vX.Y.Z AGENTARCH_BIN_DIR=path sh scripts/install.sh"
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
sha256_of() {
  if [ "$hash_program" = sha256sum ]; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

# Without a local cosign, fetch the official Sigstore build pinned below by SHA-256, use it from
# the temporary directory for this one verification, and delete it with everything else. The pin
# lives in this script, so it is exactly as trusted as the installer you are already running.
COSIGN_VERSION=v3.1.4
cosign_pin() {
  case "$1" in
    linux_amd64) echo 424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa ;;
    linux_arm64) echo ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2 ;;
    darwin_amd64) echo 9d1e4abe2b6be279ed0d2ff706fde1ccd738747c834e3e6d44039c238867334f ;;
    darwin_arm64) echo 9f9b8cd557cf2e5b3df3373a00baa005a86b02391e3e06dd0ce37b3a1533d3a4 ;;
    *) return 1 ;;
  esac
}
if command -v cosign >/dev/null 2>&1; then
  cosign=cosign
else
  pin=$(cosign_pin "${os}_${arch}") || die "no pinned cosign for ${os}/${arch}; install cosign and retry"
  say "cosign not found; fetching the pinned Sigstore cosign $COSIGN_VERSION for this install only…"
  cosign="$tmp/cosign"
  download "https://github.com/sigstore/cosign/releases/download/$COSIGN_VERSION/cosign-${os}-${arch}" "$cosign" ||
    die "could not download cosign; install it from https://docs.sigstore.dev/cosign/system_config/installation/ and retry"
  [ "$(sha256_of "$cosign")" = "$pin" ] || die "downloaded cosign does not match its pinned SHA-256; nothing was installed"
  chmod 700 "$cosign"
fi

version=${AGENTARCH_VERSION:-}
if [ -z "$version" ]; then
  say "resolving the latest release…"
  # The releases/latest page redirects to the tag. Unlike the REST API it has no anonymous rate
  # limit, which shared CI runners and office NATs exhaust. The tag is validated below either way.
  latest=$(curl --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 --max-time 60 -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
    die "could not resolve latest release; set AGENTARCH_VERSION=vX.Y.Z"
  case "$latest" in
    "https://github.com/$REPO/releases/tag/"*) version=${latest#"https://github.com/$REPO/releases/tag/"} ;;
    *) die "could not resolve latest release; set AGENTARCH_VERSION=vX.Y.Z" ;;
  esac
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
"$cosign" verify-blob "$tmp/checksums.txt" \
  --bundle "$tmp/checksums.txt.sigstore.json" \
  --certificate-identity "$identity" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com ||
  die "release signature verification failed; nothing was installed"
want=$(awk -v name="$archive" '$2 == name { count++; digest=$1 } END { if (count != 1 || length(digest) != 64 || digest ~ /[^0-9a-f]/) exit 1; print digest }' "$tmp/checksums.txt") ||
  die "$archive must appear exactly once with a SHA-256 digest in signed checksums.txt"

say "downloading $archive…"
download "$base/$archive" "$tmp/$archive" || die "could not download $archive"
got=$(sha256_of "$tmp/$archive")
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
