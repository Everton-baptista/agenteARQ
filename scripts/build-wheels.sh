#!/usr/bin/env bash
# Build one wheel per platform, each carrying its binary.
#
# The pattern ruff uses: `pipx install agentarch` needs no compiler, no Go, and no network beyond
# PyPI itself. Nothing is fetched at install time.
set -euo pipefail

VERSION="${1:?usage: build-wheels.sh vX.Y.Z}"
VERSION="${VERSION#v}"
BASE="https://github.com/Everton-baptista/agenteARQ/releases/download/v${VERSION}"
WORK="$(mktemp -d)"
OUT="dist-pypi/dist"

# PEP 600 / PEP 425 tags.
PLATFORMS=(
  "macosx_11_0_arm64  darwin  arm64 agentarch"
  "macosx_10_9_x86_64 darwin  amd64 agentarch"
  "manylinux2014_aarch64 linux arm64 agentarch"
  "manylinux2014_x86_64  linux amd64 agentarch"
  "win_amd64          windows amd64 agentarch.exe"
)

pip install --quiet hatchling build wheel
mkdir -p "$OUT"
curl -fsSL -o "$WORK/checksums.txt" "$BASE/checksums.txt"

for row in "${PLATFORMS[@]}"; do
  read -r tag os arch exe <<< "$row"
  echo "── $tag"

  if [ "$os" = "windows" ]; then
    archive="agentarch_${VERSION}_${os}_${arch}.zip"
  else
    archive="agentarch_${VERSION}_${os}_${arch}.tar.gz"
  fi
  curl -fsSL -o "$WORK/$archive" "$BASE/$archive"
  ( cd "$WORK" && grep " ${archive}\$" checksums.txt | sha256sum -c - )

  stage="$WORK/stage-$tag"
  rm -rf "$stage"
  cp -R dist-pypi "$stage"
  rm -rf "$stage/dist"
  mkdir -p "$stage/agentarch/bin"

  case "$archive" in
    *.zip)    unzip -qo "$WORK/$archive" "$exe" -d "$stage/agentarch/bin" ;;
    *.tar.gz) tar -xzf "$WORK/$archive" -C "$stage/agentarch/bin" "$exe" ;;
  esac
  chmod +x "$stage/agentarch/bin/$exe"

  sed -i.bak "s/^__version__ = .*/__version__ = \"$VERSION\"/" "$stage/agentarch/__init__.py"
  rm -f "$stage/agentarch/__init__.py.bak"
  cp README.md "$stage/README.md"

  ( cd "$stage" && python -m build --wheel --outdir "$WORK/wheels" )

  # Retag as platform-specific. The default is pure-Python, which would let pip install a
  # macOS binary on Linux.
  for w in "$WORK/wheels"/*.whl; do
    python -m wheel tags --platform-tag "$tag" --remove "$w" >/dev/null
  done
  mv "$WORK/wheels"/*.whl "$OUT/"
done

ls -1 "$OUT"
echo "built $(ls -1 "$OUT" | wc -l) wheel(s) for $VERSION"
