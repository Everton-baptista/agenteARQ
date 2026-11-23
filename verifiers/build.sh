#!/bin/sh
# Explicit, network-enabled preparation only. Never invoked by agentarch verify.
set -eu
die() { printf '%s\n' "$1" >&2; exit 1; }
if [ "${1:-}" = --help ] || [ "$#" -ne 4 ]; then
  printf '%s\n' 'Usage: sh verifiers/build.sh <typescript-web|python-api|go-service|static-site|docs|universal> <project-dir> <base-image@sha256:digest> <output-tag>'
  printf '%s\n' 'Builds with network access; copies only reviewed dependency manifests. Does not push or run verification.'
  printf '%s\n' 'static-site and docs pin their tooling in this repository and read nothing from <project-dir>.'
  if [ "${1:-}" = --help ]; then exit 0; else exit 1; fi
fi
profile=$1
project=$2
base=$3
tag=$4
case "$profile" in typescript-web|python-api|go-service|static-site|docs|universal) ;; *) die 'Unsupported verifier profile' ;; esac
printf '%s\n' "$base" | LC_ALL=C grep -Eq '^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}$' || die 'Supply a real base image reference pinned by SHA-256 digest'
case "$base" in *'
'*) die 'Base image must be a single line' ;; esac
case "$tag" in ''|-*) die 'Supply a nonempty output image tag' ;; esac
engine=${CONTAINER_ENGINE:-}
if [ -z "$engine" ]; then
  if command -v podman >/dev/null 2>&1; then engine=podman; else engine=docker; fi
fi
case "$engine" in docker|podman) ;; *) die 'CONTAINER_ENGINE must be docker or podman' ;; esac
command -v "$engine" >/dev/null 2>&1 || die "$engine is unavailable"
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project=$(CDPATH= cd -- "$project" && pwd)
context=$(mktemp -d)
trap 'rm -rf "$context"' 0
trap 'exit 130' INT
trap 'exit 143' TERM
case "$profile" in
  typescript-web)
    for file in package.json package-lock.json; do
      [ -f "$project/$file" ] && [ ! -L "$project/$file" ] || die "Missing regular dependency input: $file"
      cp "$project/$file" "$context/$file"
    done ;;
  python-api)
    file=requirements-verifier.lock
    [ -f "$project/$file" ] && [ ! -L "$project/$file" ] || die "Missing regular dependency input: $file"
    cp "$project/$file" "$context/$file" ;;
  go-service)
    [ -f "$project/go.mod" ] && [ ! -L "$project/go.mod" ] || die "Missing regular dependency input: go.mod"
    cp "$project/go.mod" "$context/go.mod"
    if [ -f "$project/go.sum" ] && [ ! -L "$project/go.sum" ]; then cp "$project/go.sum" "$context/go.sum"; else : > "$context/go.sum"; fi ;;
  static-site|docs)
    # Tooling, its lockfile, fixed configs and wrappers come from this repository, never the project.
    if [ "$profile" = static-site ]; then
      files='package.json package-lock.json htmlvalidate.json eslint.config.mjs agentarch-html agentarch-js-lint agentarch-site-links agentarch-browser'
    else
      files='package.json package-lock.json markdownlint.json agentarch-markdown agentarch-docs-links'
    fi
    for file in $files; do cp "$here/$profile/$file" "$context/$file"; done
    mkdir "$context/content"
    cp "$here/content/agentarch-links.js" "$context/content/agentarch-links.js" ;;
  universal) ;; # the floor only; extend this image with the project's own toolchain
esac
# Every profile carries the security floor: pinned scanners, fixed configs, offline OSV data.
mkdir "$context/floor"
for file in install.sh agentarch-secrets agentarch-dependency-audit gitleaks.toml osv-scanner.toml; do
  cp "$here/floor/$file" "$context/floor/$file"
done
printf '%s\n' "Building $tag with network access from $base; no application source or host credentials are copied."
"$engine" build --network=default --build-arg "BASE_IMAGE=$base" --tag "$tag" --file "$here/$profile/Dockerfile" "$context"
printf '%s\n' 'Image built. Obtain its repository digest through your reviewed registry/import process before verification; a local tag or image config ID is insufficient.'
