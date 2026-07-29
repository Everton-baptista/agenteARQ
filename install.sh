#!/bin/sh
#
# Installs the agentarch CLI.
#
#   curl -fsSL https://raw.githubusercontent.com/Everton-baptista/agenteARQ/main/install.sh | sh
#
# POSIX sh on purpose: this has to run on a stock macOS, a Debian slim container and an Alpine
# CI image without asking any of them to acquire a shell first. The whole argument for a single
# static Go binary is that nothing needs to be installed to use it — an installer that needs Go,
# Node or Python to run would give that away at the door.
#
# What it does, in order: work out the platform, resolve the release, download the archive and
# the signed checksums.txt, verify the archive against it, and put the binary somewhere on PATH.
#
# Environment:
#   AGENTARCH_VERSION   a tag such as v0.1.2 (default: the latest release)
#   AGENTARCH_BIN_DIR   where to install (default: /usr/local/bin, else ~/.local/bin)

set -eu

REPO="Everton-baptista/agenteARQ"

die() { printf '\nerror: %s\n' "$1" >&2; exit 1; }
say() { printf '%s\n' "$1"; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required and was not found"
}

# ---------------------------------------------------------------- platform

os=$(uname -s)
arch=$(uname -m)

case "$os" in
	Darwin) os=darwin ;;
	Linux)  os=linux ;;
	MINGW*|MSYS*|CYGWIN*)
		die "on Windows, download the .zip from https://github.com/$REPO/releases
       and put agentarch.exe somewhere on your PATH" ;;
	*) die "unsupported operating system: $os" ;;
esac

case "$arch" in
	x86_64|amd64)  arch=amd64 ;;
	arm64|aarch64) arch=arm64 ;;
	*) die "unsupported architecture: $arch" ;;
esac

need curl
need tar

# ---------------------------------------------------------------- version

version=${AGENTARCH_VERSION:-}
if [ -z "$version" ]; then
	say "resolving the latest release…"
	# Parsed with sed rather than jq: requiring jq to install a zero-dependency binary is the
	# same mistake as requiring Go.
	version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
	[ -n "$version" ] || die "could not resolve the latest release.
       Set AGENTARCH_VERSION=vX.Y.Z, or check https://github.com/$REPO/releases"
fi

# Tags carry the v, artifact names do not.
plain=$(printf '%s' "$version" | sed 's/^v//')
archive="agentarch_${plain}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

# ---------------------------------------------------------------- download

tmp=$(mktemp -d)
# shellcheck disable=SC2064  # expand $tmp now, not at trap time
trap "rm -rf '$tmp'" EXIT INT TERM

say "downloading $archive ($version)…"
curl -fsSL -o "$tmp/$archive" "$base/$archive" ||
	die "no such artifact: $base/$archive
       That platform may not be built for this release. See https://github.com/$REPO/releases"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" ||
	die "could not download checksums.txt from $base"

# ---------------------------------------------------------------- verify
#
# Not optional, and not skipped when the tool is missing. A governance tool whose own installer
# shrugs at an unverified download would be arguing against itself.

if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
else
	die "neither sha256sum nor shasum is available, so the download cannot be verified"
fi

want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || die "$archive is not listed in checksums.txt"

if [ "$got" != "$want" ]; then
	die "checksum mismatch for $archive
       expected $want
       got      $got
       Nothing was installed. Do not run the downloaded file."
fi
say "checksum ok"

# ---------------------------------------------------------------- install

bindir=${AGENTARCH_BIN_DIR:-}
if [ -z "$bindir" ]; then
	if [ -w /usr/local/bin ] 2>/dev/null; then
		bindir=/usr/local/bin
	else
		# Never sudo on the user's behalf. An installer that silently escalates is a habit worth
		# not teaching, so fall back to a directory that needs no privileges.
		bindir="$HOME/.local/bin"
	fi
fi
mkdir -p "$bindir" || die "cannot create $bindir"

tar -xzf "$tmp/$archive" -C "$tmp" agentarch || die "could not extract agentarch from $archive"
mv "$tmp/agentarch" "$bindir/agentarch"
chmod +x "$bindir/agentarch"

say ""
say "installed $("$bindir/agentarch" version | head -n 1) to $bindir/agentarch"

# ---------------------------------------------------------------- PATH

case ":$PATH:" in
	*":$bindir:"*) onpath=yes ;;
	*) onpath=no ;;
esac

if [ "$onpath" = no ]; then
	shellrc="$HOME/.profile"
	case "${SHELL:-}" in
		*zsh)  shellrc="$HOME/.zshrc" ;;
		*bash) shellrc="$HOME/.bashrc" ;;
	esac
	say ""
	say "$bindir is not on your PATH. Add it:"
	say ""
	say "  echo 'export PATH=\"\$PATH:$bindir\"' >> $shellrc"
	say "  export PATH=\"\$PATH:$bindir\""
fi

cat <<'EOF'

Next, in the directory you want the agent to live in:

  agentarch start

That asks what you are building and does the rest.

EOF

say "The signature on checksums.txt can be verified independently:"
say ""
say "  cosign verify-blob checksums.txt --signature checksums.txt.sig \\"
say "    --certificate checksums.txt.pem \\"
say "    --certificate-identity-regexp 'https://github.com/$REPO/.*' \\"
say "    --certificate-oidc-issuer https://token.actions.githubusercontent.com"
say ""
say "This script checked the archive against that file over HTTPS, which catches a corrupt or"
say "substituted download. It does not by itself prove who built it — the cosign check does."
