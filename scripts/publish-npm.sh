#!/usr/bin/env bash
# Publish the launcher and one package per platform.
#
# The launcher declares the platform packages as optionalDependencies, the pattern esbuild and
# swc use: npm installs only the one that matches. There is no postinstall script downloading
# anything — a postinstall that fetches an executable is a supply-chain step that runs with the
# developer's credentials and is invisible in a lockfile, which would be a poor thing for a tool
# whose central argument is that a pack must be inert data.
set -euo pipefail

VERSION="${1:?usage: publish-npm.sh vX.Y.Z}"
VERSION="${VERSION#v}"
BASE="https://github.com/Everton-baptista/agenteARQ/releases/download/v${VERSION}"
WORK="$(mktemp -d)"

PLATFORMS=(
  "darwin-arm64 darwin arm64 agentarch"
  "darwin-x64   darwin amd64 agentarch"
  "linux-arm64  linux  arm64 agentarch"
  "linux-x64    linux  amd64 agentarch"
  "win32-x64    windows amd64 agentarch.exe"
)

echo "fetching checksums"
curl -fsSL -o "$WORK/checksums.txt" "$BASE/checksums.txt"

for row in "${PLATFORMS[@]}"; do
  read -r pkgsuffix os arch exe <<< "$row"
  name="@agentarch/cli-${pkgsuffix}"
  dir="$WORK/$pkgsuffix"
  mkdir -p "$dir/bin"

  if [ "$os" = "windows" ]; then
    archive="agentarch_${VERSION}_${os}_${arch}.zip"
  else
    archive="agentarch_${VERSION}_${os}_${arch}.tar.gz"
  fi

  echo "── $name"
  curl -fsSL -o "$WORK/$archive" "$BASE/$archive"

  # Verify before unpacking, every time. Publishing an artifact nobody checked would undo the
  # signing in the release job.
  ( cd "$WORK" && grep " ${archive}\$" checksums.txt | sha256sum -c - )

  case "$archive" in
    *.zip)    unzip -qo "$WORK/$archive" "$exe" -d "$dir/bin" ;;
    *.tar.gz) tar -xzf "$WORK/$archive" -C "$dir/bin" "$exe" ;;
  esac
  chmod +x "$dir/bin/$exe"

  node_os="$os"; [ "$os" = "windows" ] && node_os="win32"
  node_arch="$arch"; [ "$arch" = "amd64" ] && node_arch="x64"

  cat > "$dir/package.json" <<JSON
{
  "name": "$name",
  "version": "$VERSION",
  "description": "agentarch binary for $node_os $node_arch",
  "license": "Apache-2.0",
  "repository": { "type": "git", "url": "git+https://github.com/Everton-baptista/agenteARQ.git" },
  "os": ["$node_os"],
  "cpu": ["$node_arch"],
  "files": ["bin/"]
}
JSON
  # --provenance signs the tarball with the workflow and commit that built it, so a consumer can
  # verify the chain from npm back to this repository rather than trusting the registry alone.
  npm publish "$dir" --access public --provenance
done

echo "── agentarch (launcher)"
LAUNCHER="$WORK/launcher"
cp -R dist-npm "$LAUNCHER"
# One version for every package, so a partially-published release is visibly partial.
node -e "
  const fs = require('fs'), p = '$LAUNCHER/package.json';
  const j = JSON.parse(fs.readFileSync(p));
  j.version = '$VERSION';
  for (const k of Object.keys(j.optionalDependencies)) j.optionalDependencies[k] = '$VERSION';
  fs.writeFileSync(p, JSON.stringify(j, null, 2) + '\n');
"
npm publish "$LAUNCHER" --access public --provenance

echo "published agentarch@$VERSION and ${#PLATFORMS[@]} platform packages"
