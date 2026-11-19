# Prepare images for isolated verification

`agentarch workflow verify` and `agentarch verify-profile` execute checks in a **previously
installed image identified by repository digest**. Neither command installs project dependencies,
pulls an image, or falls back to running on the host. These recipes prepare that image explicitly:

| Profile | Recipe | Dependency inputs |
|---|---|---|
| `typescript-web` | [`verifiers/typescript-web/Dockerfile`](../verifiers/typescript-web/Dockerfile) | `package.json`, `package-lock.json` |
| `python-api` | [`verifiers/python-api/Dockerfile`](../verifiers/python-api/Dockerfile) | `requirements-verifier.lock` |
| `go-service` | [`verifiers/go-service/Dockerfile`](../verifiers/go-service/Dockerfile) | `go.mod`, `go.sum` |
| `static-site` | [`verifiers/static-site/Dockerfile`](../verifiers/static-site/Dockerfile) | none from the project; tooling pinned in `verifiers/static-site/package-lock.json` |
| `docs` | [`verifiers/docs/Dockerfile`](../verifiers/docs/Dockerfile) | none from the project; tooling pinned in `verifiers/docs/package-lock.json` |
| `universal` | [`verifiers/universal/Dockerfile`](../verifiers/universal/Dockerfile) | none; the base supplies the project's toolchain |

Every recipe installs the **security floor** from [`verifiers/floor/`](../verifiers/floor/):
gitleaks 8.30.1 and osv-scanner 2.7.0, each verified by SHA-256, the image's own scanner
configuration, and the OSV vulnerability database for PyPI, npm, Go, crates.io, RubyGems,
Packagist, Maven, NuGet, Pub and Hex. The database is a snapshot: its download time and each
archive's SHA-256 are recorded in `/opt/agentarch-floor/osv/`, and the audit refuses to pass
with a snapshot older than 14 days (`AGENTARCH_OSV_MAX_AGE_DAYS`). Rebuild the image to refresh it.
The base needs `tar`, a SHA-256 tool and one of curl, wget, python3 or node during the build.

The image build is a separate, network-enabled operation that downloads packages and executes
build instructions. Review the base image, dependency inputs and Dockerfile before building.
Package code is later executed by the checks; a digest or lockfile does not establish that code
is trustworthy. Never include credentials in a Dockerfile, build argument or dependency URL.
Private packages require a separately reviewed build-secret setup; these recipes do not copy
`.npmrc`, pip configuration, `.env`, SSH keys or application source.

## Select and pin a base

Use a supported Node image for TypeScript, or a Playwright image when browser tests need its
preinstalled browsers and OS libraries. The Playwright package in `package-lock.json` must match
the selected browser-image version. For Python, use a compatible global-Python image containing
`python` and pip. Both bases must contain `/bin/sh`, `tar` and `grep`; they must not declare
volumes that introduce extra writable paths. The recipes install as the container root user;
verification still uses the coordinator's read-only root and restricted container settings.

Select an actual publisher/version and review its registry digest. The following **explicit
preparation** uses your selected value; it does not prescribe a moving version or invent a digest:

```sh
# Set this to the exact publisher/version you reviewed, e.g. a Node, Playwright or Python tag.
: "${SELECTED_BASE:?Set the reviewed base image tag}"
docker pull "$SELECTED_BASE"
BASE_IMAGE=$(docker image inspect "$SELECTED_BASE" --format '{{index .RepoDigests 0}}')
printf '%s\n' "$BASE_IMAGE"
```

Record and review the resulting `repository@sha256:...`, including the intended platform.
For a previously obtained digest, set `BASE_IMAGE` directly. The build helper rejects an
unpinned base **before** starting Docker/Podman; the Dockerfiles have no default and check it
again. A direct `docker build` may resolve `FROM` before executing that check, so use the helper.

## TypeScript web

This initial recipe supports one npm package at the project root. Commit its reviewed lockfile,
including development dependencies. Monorepos, local `file:` dependencies, pnpm/Yarn layouts,
and dependencies requiring project source during installation need a dedicated recipe.

```sh
# Run from this AgenteARQ checkout; PRODUCT is your software repository.
: "${PRODUCT:?Set the product repository directory}"
: "${BASE_IMAGE:?Set a reviewed base repository@sha256:digest}"
: "${VERIFIER_TAG:?Set the output image tag you own}"
sh verifiers/build.sh typescript-web "$PRODUCT" "$BASE_IMAGE" "$VERIFIER_TAG"
```

The recipe runs `npm ci --include=dev --ignore-scripts`, then places dependencies in
`/node_modules` and adds `/node_modules/.bin` to `PATH`. Node resolves packages there by walking
up from `/workspace`. It does **not** install into `/workspace/node_modules`: the runner mounts
a fresh tmpfs on `/workspace`, hiding anything built there. Dependencies are read-only during
checks; configure tool caches in `/tmp` or `/workspace`, not inside `/node_modules`.

Install lifecycle scripts are deliberately disabled. If a dependency needs compilation or
generation, add a reviewed package-specific step to the Dockerfile before moving `node_modules`.
That step executes dependency code with the build's network access. Do not expect a missing
native binary to be downloaded during verification. Playwright browser downloads are disabled;
the selected base must already provide compatible binaries outside `/workspace` and `/tmp`.
Preserve the base's browser-path configuration. The recipe does not download browsers itself.

The profile invokes these exact commands from the source root:

```text
npm run lint
npm run typecheck
npm run test -- --run
npm run build
npm run test:e2e
npm run test:a11y
npm run test:security
```

Define every script and make tests terminate without watch mode. Each command gets a **new**
source copy and container: `test:e2e` cannot reuse output or a web server from the earlier
`build` command. Start its own local server and prepare its own assets, or use a dev server.
Browser tests must fit the current 1 GiB memory, 128-process and tmpfs limits; restrict workers.
The runner does not add host IPC or extra capabilities to satisfy a browser configuration.
`test:security` must perform offline tests. `npm audit` requires registry access and cannot be a
passing offline check; the floor's `dependency-audit` covers known vulnerabilities offline instead.

## Python API

Prepare a fully resolved, hashed pip requirements file for the base's Python version and
platform. Include the product's runtime dependencies plus `ruff`, `mypy`, `pytest` and `bandit`.
For a project already using `requirements.txt`, one preparation flow with an installed, reviewed
`uv` tool is:

```sh
cd "$PRODUCT"
printf '%s\n' '-r requirements.txt' 'ruff' 'mypy' 'pytest' 'bandit' > requirements-verifier.in
uv pip compile --generate-hashes --output-file requirements-verifier.lock requirements-verifier.in
# Review and commit the resolved versions/hashes; this resolution step uses the network.
```

If your project uses `pyproject.toml`, export its runtime and development dependencies with your
existing lock workflow instead. Do not overwrite an existing verifier input blindly. The final
lock must include transitive dependencies and hashes; plain floating requirements are rejected.
The recipe requires wheels and does not build source distributions. A package without a wheel
for your platform needs a separate, reviewed wheel-build process.

```sh
# Back in the AgenteARQ checkout, with PRODUCT/BASE_IMAGE/VERIFIER_TAG set as above:
sh verifiers/build.sh python-api "$PRODUCT" "$BASE_IMAGE" "$VERIFIER_TAG"
```

Dependencies are installed in the image's **global Python environment**, never in a virtualenv
under `/workspace`. The build checks that all four tools are importable and `pip check` passes.
The profile subsequently invokes:

```text
python -m ruff check .
python -m mypy .
python -m pytest -q
python -m pip check
python -m bandit -r . -q -x './.venv/*,./tests/*'
```

The Bandit exclusions cover only the root `.venv/` and `tests/` directories and
their descendants. Application paths such as `src/tests/`, `tests_support/` and
managed skills remain scanned; `B101` is not disabled globally. Keep pytest tests
under the declared root `tests/` directory. Bandit 1.9.4 discovers paths with a
`./` prefix when scanning `.`; bare directory exclusions can become nonmatching
globs. The explicit root globs follow its [documented `-x` interface](https://bandit.readthedocs.io/en/1.9.4/man/bandit.html)
and [file-discovery implementation](https://github.com/PyCQA/bandit/blob/1.9.4/bandit/core/manager.py).

To run the real-container boundary regression with an installed Python verifier:

```sh
AGENTARCH_TEST_PYTHON_VERIFIER_IMAGE="$VERIFIER_IMAGE" \
  go test ./internal/workflow -run TestPythonBanditScopeOCI -count=1 -v
```

The regression checks excluded pytest assertions and requires `B101` findings in
application code, nested application test directories and managed skill scripts.

Configure your source/import layout and tool exclusions in the project. A `src/` layout may
need pytest's `pythonpath` setting or a reviewed `PYTHONPATH=/workspace/src` in the image;
verification does not run `pip install -e .`. Use local fixtures such as SQLite where suitable.
External databases and API services are unreachable; a test can start local processes within
its own container and command. `pip check` checks dependency consistency, not vulnerability data; the floor's
`dependency-audit` checks vulnerability data.

## Static site and docs

These two profiles check content, not an application build, so their images carry only
AgenteARQ's own pinned tooling. `build.sh` copies `package.json`, `package-lock.json`, the fixed
configuration and the wrappers from `verifiers/<profile>/`, plus the shared link checker
[`verifiers/content/agentarch-links.js`](../verifiers/content/agentarch-links.js) (Node, no
dependencies); it reads nothing from the project directory. Use a Node base such as
`node:<version>-bookworm-slim`, pinned by digest; it needs `node`, `npm`, `/bin/sh`, `tar` and
`grep`, and the floor downloads through `node` when curl and wget are absent.

```sh
# Run from this AgenteARQ checkout. The project argument is required but not read.
: "${BASE_IMAGE:?Set a reviewed Node repository@sha256:digest}"
: "${VERIFIER_TAG:?Set the output image tag you own}"
sh verifiers/build.sh static-site . "$BASE_IMAGE" "$VERIFIER_TAG"   # or: docs
```

| Profile | Check | Program | Tool and fixed configuration |
|---|---|---|---|
| `static-site` | `html` | `agentarch-html` | html-validate 11.18.0, presets `standard` and `a11y`, plus `missing-doctype` and `no-missing-references` ([`htmlvalidate.json`](../verifiers/static-site/htmlvalidate.json)) |
| `static-site` | `lint` | `agentarch-js-lint` | ESLint 10.15.0 with `@eslint/js` 10.0.1 recommended rules and `globals` 17.14.0 browser globals ([`eslint.config.mjs`](../verifiers/static-site/eslint.config.mjs)) |
| `static-site` | `links` | `agentarch-site-links` | `agentarch-links.js html` |
| `docs` | `markdown` | `agentarch-markdown` | markdownlint 0.42.1 as pinned by markdownlint-cli2 0.24.1; line length and purely stylistic rules off, structural rules on ([`markdownlint.json`](../verifiers/docs/markdownlint.json)) |
| `docs` | `links` | `agentarch-docs-links` | `agentarch-links.js markdown` |

The wrappers load only the image's configuration. html-validate runs through a static config
loader, ESLint through an override config file with no lookup, and markdownlint through its rule
engine rather than the markdownlint-cli2 command line, which would merge the project's
`.markdownlint*` and `.markdownlint-cli2.*` files over `--config` and can load custom rules (code)
from them. Inline comments that switch a rule off (`[html-validate-disable-*]`, `eslint-disable*`,
`/* eslint rule: off */`, `markdownlint-disable`) are not applied; `/* global name */` still
declares what classic scripts share. ESLint treats `*.mjs` as modules and `*.js` as classic
scripts, unless the file contains `import`/`export` statements or a page loads it with
`<script type="module">`. Classic scripts report only unused local variables, since their
top-level functions may be called from other scripts or `onclick=""`.

`links` never fetches a URL: anything with a scheme (`https:`, `mailto:`, `data:` ...), `//host`
references, fragment-only links and template placeholders are skipped, and `#fragment` and
`?query` parts are dropped before resolving. A root-relative path (`/a/b`) resolves against the
project root, a path that leaves the project is broken, and in HTML a directory link needs an
`index.html`. Markdown inside fenced code blocks, inline code and HTML comments is not read.
No `.html` file (static site) or no `.md` file (docs) fails both content checks.

Each profile has fixtures in `verifiers/<profile>/fixtures/` (`pass` and `fail-*`).
`scripts/test-content-profiles-live.sh` commits each to a throwaway repository and verifies it
with the real image:

```sh
PROFILE=static-site IMAGE="$VERIFIER_IMAGE" AGENTARCH_BIN=/path/to/agentarch \
  bash scripts/test-content-profiles-live.sh
```

## Install the final digest, then verify

The helper builds a local tag without publishing it. Inspect the resulting local image first:

```sh
docker image inspect "$VERIFIER_TAG" --format '{{range .RepoDigests}}{{println .}}{{end}}'
```

A containerd-backed Docker image store can expose `RepoDigests` immediately after a local
build or import. Classic image stores may leave that list empty. If a reviewed
`repository@sha256:...` already resolves in the verifier's local image store, it is usable
without a push or pull:

```sh
# Select the intended repository/platform from the inspected RepoDigests.
: "${VERIFIER_IMAGE:?Set the reviewed repository@sha256:digest}"
docker image inspect "$VERIFIER_IMAGE" >/dev/null
```

`RepoDigests` refers to repository manifests or image indexes. A configuration-object digest
identifies different content. Do not construct a repository digest from `.Id`: the image-ID
representation varies with the image store, and that field alone is not a verified repository
reference. Review the manifest/index and intended platform as well as local availability.

If no suitable repository digest is available locally, use your reviewed import or registry
process to install one. For an image you have authorized for publication to your own registry,
the separate preparation steps can include:

```sh
# Publishing is a separate action; run only for your chosen registry and reviewed image.
docker push "$VERIFIER_TAG"
docker image inspect "$VERIFIER_TAG" --format '{{range .RepoDigests}}{{println .}}{{end}}'
# Set VERIFIER_IMAGE to the reviewed digest; pull it on the verifier machine if absent there.
: "${VERIFIER_IMAGE:?Set the reviewed repository@sha256:digest}"
docker pull "$VERIFIER_IMAGE"
docker image inspect "$VERIFIER_IMAGE" >/dev/null
```

Only the registry preparation path needs those network operations. Verification uses `--pull=never` and
`--network=none`, a read-only root, dropped capabilities, bounded writable `/workspace` and
`/tmp`, and no host directory mounts. The runner replaces image entrypoints; an entrypoint
cannot be relied on to install dependencies or enforce a lock check. Rebuild and repin when
dependency inputs change: automatic matching of the image's lockfile to project lockfiles is
not currently implemented. A missing executable or incompatible image blocks readiness.

## Run the profile with or without workflow state

For an initialized product whose implementation tasks are complete, run the workflow gate:

```sh
agentarch workflow verify --root "$PRODUCT" --image "$VERIFIER_IMAGE" --format json
```

That command also evaluates declared development executors and application agents, records
workflow evidence, and can advance the product to independent review.

To measure the **same fixed engineering checks and container isolation** without installing
a plugin or initializing a workflow, use:

```sh
agentarch verify-profile --root "$PRODUCT" --profile typescript-web \
  --image "$VERIFIER_IMAGE" --format json
# Use --profile python-api for the Python checks listed above.
```

`PRODUCT` must be a Git worktree root. `verify-profile` accepts any built-in profile except `universal`;
it needs no product manifest, tasks, skills, or workflow state and does not create them. It
returns the source fingerprint and check results on stdout. Save JSON outside the measured
worktree to avoid changing its source fingerprint during verification. A source change during
the run makes the measurement inconclusive. Failed or inconclusive checks return exit 4.

This standalone measurement does not run the agent policy gate, record independent review,
advance workflow state, or certify PR readiness. It supports measuring an executor-only
baseline with the same engineering checks used for a product following the AgenteARQ workflow.

The recipes are preparation examples, not published verifier images or proof that all browser
and application checks pass. Run the complete profile with your actual image before relying
on its evidence. See [the workflow isolation contract](workflow.md#isolated-verification).

Primary references: [Dockerfile ARG/FROM](https://docs.docker.com/reference/dockerfile/#understand-how-arg-and-from-interact),
[Docker containerd image store](https://docs.docker.com/engine/storage/containerd/),
[Docker image and manifest digests](https://docs.docker.com/dhi/explore/security-concepts/digests/),
[npm ci](https://docs.npmjs.com/cli/v11/commands/npm-ci/),
[Node package resolution](https://nodejs.org/api/modules.html#loading-from-node_modules-folders),
[pip secure installation](https://pip.pypa.io/en/stable/topics/secure-installs/),
[Playwright container requirements](https://playwright.dev/docs/docker).
