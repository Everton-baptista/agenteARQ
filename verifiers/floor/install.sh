#!/bin/sh
# Installs the AgenteARQ security floor into a verifier image. Run only during an explicit,
# network-enabled image build (verifiers/build.sh); verification itself has no network.
#
# Installs two scanners pinned by version and SHA-256, the offline OSV vulnerability database,
# and the two wrappers every engineering profile calls: agentarch-secrets and
# agentarch-dependency-audit. Needs /bin/sh, tar, a SHA-256 tool, and one downloader among
# curl, wget, python3 or node.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
prefix=/opt/agentarch-floor
bindir=/usr/local/bin

die() { printf 'floor: %s\n' "$1" >&2; exit 1; }

# Pinned tools. Bump the version and both checksums together, from the project's release page.
GITLEAKS_VERSION=8.30.1
GITLEAKS_SHA256_X64=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
GITLEAKS_SHA256_ARM64=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080
OSV_SCANNER_VERSION=2.7.0
OSV_SCANNER_SHA256_AMD64=160c6253e2922e9e0a6a9771c5c4c40a4b3c6090b34a3cdd5e80c96cc2e4218b
OSV_SCANNER_SHA256_ARM64=4061ef4c021ed3c47f203b818f337e351336cc5cd67c0274fc36bd1dfc728dd7

# Vulnerability data is a moving snapshot, so it cannot be pinned by checksum. Each archive's
# SHA-256 and the download time are recorded in the image so a verification can be traced.
OSV_ECOSYSTEMS="PyPI npm Go crates.io RubyGems Packagist Maven NuGet Pub Hex"

fetch() { # url, destination
  if command -v curl >/dev/null 2>&1; then
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$2" "$1"
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c 'import shutil,sys,urllib.request
with urllib.request.urlopen(sys.argv[1], timeout=300) as r, open(sys.argv[2], "wb") as f:
    shutil.copyfileobj(r, f)' "$1" "$2"
  elif command -v node >/dev/null 2>&1; then
    # Streams to disk (the OSV archives are large) and retries a dropped connection, like curl --retry 3.
    node -e 'const fs=require("fs"),{Readable}=require("stream"),{pipeline}=require("stream/promises");const [u,d]=process.argv.slice(1);(async()=>{for(let i=1;;i++){try{const r=await fetch(u);if(!r.ok)throw new Error("HTTP "+r.status);await pipeline(Readable.fromWeb(r.body),fs.createWriteStream(d));return}catch(e){if(i>3){console.error(e);process.exit(1)}console.error("floor: retry "+i+" for "+u+": "+e.message);await new Promise(s=>setTimeout(s,5000*i))}}})()' "$1" "$2"
  else
    die "no downloader available (curl, wget, python3 or node)"
  fi
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  elif command -v python3 >/dev/null 2>&1; then python3 -c 'import hashlib,sys;print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$1"
  else die "no SHA-256 tool available"; fi
}

verify() { # file, expected
  got=$(sha256_of "$1")
  [ "$got" = "$2" ] || die "checksum mismatch for $(basename "$1"): got $got"
}

case $(uname -m) in
  x86_64|amd64) gl_arch=x64; gl_sum=$GITLEAKS_SHA256_X64; osv_arch=amd64; osv_sum=$OSV_SCANNER_SHA256_AMD64 ;;
  aarch64|arm64) gl_arch=arm64; gl_sum=$GITLEAKS_SHA256_ARM64; osv_arch=arm64; osv_sum=$OSV_SCANNER_SHA256_ARM64 ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' 0

# osv-scanner 2.7.0 matches through osv-scalibr, which reads {dir}/osv-scalibr/{ecosystem}/all.zip
# (enricher/vulnmatch/osvlocal/matcher.go), not the osv-scanner/ layout its docs still describe.
# The directory must exist at build time: verification runs on a read-only root filesystem.
db="$prefix/osv/osv-scalibr"
mkdir -p "$db" "$prefix/no-ignore"

gl=gitleaks_${GITLEAKS_VERSION}_linux_${gl_arch}.tar.gz
fetch "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/${gl}" "$work/$gl"
verify "$work/$gl" "$gl_sum"
tar -xzf "$work/$gl" -C "$work" gitleaks
install -m 0755 "$work/gitleaks" "$bindir/gitleaks"

osv=osv-scanner_linux_${osv_arch}
fetch "https://github.com/google/osv-scanner/releases/download/v${OSV_SCANNER_VERSION}/${osv}" "$work/$osv"
verify "$work/$osv" "$osv_sum"
install -m 0755 "$work/$osv" "$bindir/osv-scanner"

: > "$prefix/osv/manifest.txt"
for eco in $OSV_ECOSYSTEMS; do
  mkdir -p "$db/$eco"
  fetch "https://osv-vulnerabilities.storage.googleapis.com/${eco}/all.zip" "$db/$eco/all.zip"
  printf '%s  %s\n' "$(sha256_of "$db/$eco/all.zip")" "$eco/all.zip" >> "$prefix/osv/manifest.txt"
done
date -u +%s > "$prefix/osv/downloaded-at"

install -m 0644 "$here/gitleaks.toml" "$prefix/gitleaks.toml"
install -m 0644 "$here/osv-scanner.toml" "$prefix/osv-scanner.toml"
install -m 0755 "$here/agentarch-secrets" "$bindir/agentarch-secrets"
install -m 0755 "$here/agentarch-dependency-audit" "$bindir/agentarch-dependency-audit"
chmod -R a+rX "$prefix"

"$bindir/gitleaks" version >/dev/null
"$bindir/osv-scanner" --version >/dev/null
printf 'floor: gitleaks %s, osv-scanner %s, OSV data for %s\n' "$GITLEAKS_VERSION" "$OSV_SCANNER_VERSION" "$OSV_ECOSYSTEMS"
