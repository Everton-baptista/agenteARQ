"""Launch the bundled binary.

The wheel is built per platform, so the binary beside this file is the right one. Nothing is
downloaded at install time: a postinstall that fetches an executable is a supply-chain step that
runs with the developer's credentials and is invisible in a lockfile — a poor thing for a tool
whose central argument is that a pack must be inert data.
"""

from __future__ import annotations

import os
import pathlib
import sys


def binary_path() -> pathlib.Path:
    name = "agentarch.exe" if sys.platform == "win32" else "agentarch"
    return pathlib.Path(__file__).parent / "bin" / name


def main() -> int:
    exe = binary_path()
    if not exe.exists():
        sys.stderr.write(
            f"agentarch: no binary at {exe}\n\n"
            "This wheel was built for a different platform, or the install was\n"
            "incomplete. Build from source instead:\n"
            "  go install github.com/Everton-baptista/agenteARQ/cmd/agentarch@latest\n"
        )
        return 1

    args = [str(exe), *sys.argv[1:]]
    if sys.platform == "win32":
        import subprocess

        # Exit codes are part of the contract; pass them through unchanged.
        return subprocess.run(args).returncode

    # execv replaces this process, so signals and the exit code reach the caller directly
    # rather than through a Python wrapper that might reinterpret them.
    os.execv(str(exe), args)
    return 1  # unreachable


if __name__ == "__main__":
    raise SystemExit(main())
