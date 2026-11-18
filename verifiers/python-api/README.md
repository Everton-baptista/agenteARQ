# Python benchmark verifier

`benchmark.Dockerfile` prepares the Python toolchain and Chromium headless shell required by
benchmark v2. Preparation has network access; verification uses the completed
image by digest with `--pull=never`, `--network=none`, a read-only root filesystem
and disposable `/workspace` and `/tmp` filesystems.

The Python 3.12 base must be supplied by its verified digest. All 21 Python
packages, including Ruff 0.17.5, mypy 2.4.1, pytest 9.1.1, Bandit 1.9.4 and
Playwright 1.65.1, are pinned with hashes in `requirements-benchmark.lock`.
PyYAML and its type stubs support the managed skill helpers. Python packages live
globally under `/usr/local`; Chromium lives under `/ms-playwright`. Neither is
hidden by the verifier's `/workspace` tmpfs.

Build from the repository root, after selecting and reviewing a Python base:

```sh
export PYTHON_BASE='docker.io/library/python@sha256:<verified-64-hex-digest>'
tar -C verifiers/python-api -cf - benchmark.Dockerfile requirements-benchmark.lock |
  docker build --network=default --build-arg BASE_IMAGE="$PYTHON_BASE" \
    --tag agentarch-benchmark-python:20261116 --file benchmark.Dockerfile -
docker image inspect agentarch-benchmark-python:20261116 --format '{{json .RepoDigests}}'
```

This uses a tar stream and works with a remote Docker daemon. The build downloads
Debian libraries and the Chromium headless shell revision selected by the pinned Playwright
package. Those downloads are not asserted to be reproducible from the base and
Python lock alone: retain the resulting OCI digest and package inventory for the
experiment. `libatomic1` is needed before invoking the Playwright driver on arm64.
Package installation and browser installation run only during explicit preparation.

The small fixture in `fixtures/browser_smoke` checks a real local HTTP page through
Chromium: document metadata, programmatic label, keyboard form submission and
visible status feedback. Run it through the same restrictions as verification:

```sh
export VERIFIER_IMAGE='agentarch-benchmark-python@sha256:<built-OCI-digest>'
tar -C verifiers/python-api/fixtures/browser_smoke --owner=0 --group=0 --numeric-owner -cf - . |
  docker run --rm --pull=never --network=none --read-only --cap-drop=ALL \
    --security-opt=no-new-privileges --pids-limit=128 --memory=1g --cpus=2 \
    --tmpfs /workspace:rw,nosuid,nodev,size=512m \
    --tmpfs /tmp:rw,nosuid,nodev,size=128m --workdir /workspace \
    --env HOME=/tmp --env CI=true \
    --entrypoint /bin/sh -i "$VERIFIER_IMAGE" \
    -c 'tar -xf - -C /workspace && python -m pytest -q -s'
```

This smoke confirms toolchain execution; it is not evidence that a generated
product passes its functional or accessibility requirements.
The GNU tar flags normalize archive ownership for execution with all Linux
capabilities dropped; verification must not require permission to change owners.

## Managed skill scanners

Ruff and Bandit recursively inspect both `.agents/skills` and
`agentarch/std/skills`. `mypy .` skips hidden directories but covers the canonical
copies under `agentarch/std/skills`; an explicit `mypy .agents/skills` checks the
installed copy independently. Do not exclude the entire managed namespace to
make an application pass.

The two subprocess helpers resolve an operator-selected CLI to an absolute path,
pass fixed options and separate arguments, disable the shell and bound execution
to 60 seconds. Narrow `nosec B404` / `nosec B603` annotations cover these reviewed
uses only. No other Bandit checks are disabled. The upgrade preview uses a private
temporary directory and cleans it even on failure, preserving any existing user
directory with the old preview name. Regression tests live in
`tests/test_managed_skills.py`.

The profile's security command is
`python -m bandit -r . -q -x './.venv/*,./tests/*'`. These globs exclude only root
dependency/test directories, including descendants. They do not skip `B101` in
application code or exclude nested application directories such as `src/tests`.
The original browser smoke used Playwright's `expect` assertions, so it did not
exercise Python `assert` exclusions. `TestPythonBanditScopeOCI` now checks that
boundary with real Bandit subprocesses in the pinned image; see
[`docs/verifier-images.md`](../../docs/verifier-images.md).
