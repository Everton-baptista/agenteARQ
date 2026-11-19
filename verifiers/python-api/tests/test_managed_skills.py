"""Regression checks for the reviewed subprocess uses in installed skill helpers."""
from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys

import pytest

ROOT = pathlib.Path(__file__).resolve().parents[3]


def helper(relative: str):
    path = ROOT / "content/skills" / relative
    spec = importlib.util.spec_from_file_location(path.stem, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    # Embedded content must not acquire test-generated __pycache__ artifacts.
    with pytest.MonkeyPatch.context() as patch:
        patch.setattr(sys, "dont_write_bytecode", True)
        spec.loader.exec_module(module)
    return module


def fixture_cli(tmp_path: pathlib.Path) -> pathlib.Path:
    binary = tmp_path / "installed-agentarch"
    binary.write_text("fixture executable selected by the test\n")
    return binary


def test_collect_preserves_gate_failure_and_passes_paths_as_arguments(tmp_path, monkeypatch, capsys):
    module = helper("agentarch-review-agent/scripts/collect_context.py")
    project = tmp_path / "project; literal path"
    agent = project / "agentarch/project/agents/review-me"
    agent.mkdir(parents=True)
    (agent / "agent.yaml").write_text("agent: {id: review-me}\n")
    binary = fixture_cli(tmp_path)
    calls = []

    def run(argv, **kwargs):
        calls.append((argv, kwargs))
        return subprocess.CompletedProcess(argv, 4, stdout='{"blocked": true}', stderr="")

    monkeypatch.setattr(module.subprocess, "run", run)
    monkeypatch.setattr(sys, "argv", ["collect_context.py", "review-me", "--root", str(project), "--agentarch-bin", str(binary)])
    assert module.main() == 0  # collection succeeded, not the gate
    argv, options = calls[0]
    assert argv[0] == str(binary.resolve())
    assert argv[-2:] == ["--root", str(project.resolve())]
    assert options["shell"] is False and options["check"] is False and options["timeout"] == 60
    assert "gate exit status: 4" in capsys.readouterr().out


@pytest.mark.parametrize("agent_id", ["../outside", "--root", "agent;touch-pwned"])
def test_collect_rejects_agent_path_or_command_instead_of_slug(tmp_path, monkeypatch, agent_id):
    module = helper("agentarch-review-agent/scripts/collect_context.py")
    monkeypatch.setattr(sys, "argv", ["collect_context.py", agent_id, "--root", str(tmp_path)])
    with pytest.raises(SystemExit):
        module.main()


@pytest.mark.parametrize("fail", [False, True])
def test_upgrade_preview_preserves_existing_directory_and_cleans_private_temp(tmp_path, monkeypatch, fail):
    module = helper("agentarch-upgrade-review/scripts/diff_controls.py")
    pack = tmp_path / "agentarch/std/packs/example/pack.yaml"
    pack.parent.mkdir(parents=True)
    pack.write_text("pack: {id: example, version: '1.0', requires: [{control: example, severity: major}]}\n")
    existing = tmp_path / ".agentarch-upgrade-preview"
    existing.mkdir()
    precious = existing / "user-work.txt"
    precious.write_text("keep")
    binary = fixture_cli(tmp_path)
    staged = []

    def run(argv, **kwargs):
        path = pathlib.Path(argv[-1])
        staged.append(path)
        assert argv[:3] == [str(binary.resolve()), "init", "--root"]
        assert kwargs["shell"] is False and kwargs["check"] is True and kwargs["timeout"] == 60
        assert path != existing
        if fail:
            raise subprocess.TimeoutExpired(argv, 60)
        return subprocess.CompletedProcess(argv, 0)

    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(module.subprocess, "run", run)
    monkeypatch.setattr(sys, "argv", ["diff_controls.py", "--root", str(tmp_path), "--agentarch-bin", str(binary)])
    assert module.main() == (1 if fail else 0)
    assert precious.read_text() == "keep"
    assert staged and not staged[0].exists()
