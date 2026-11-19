"""One module per provider, reached only through infra/provider.py.

Each module exposes exactly two functions and nothing else:

    client(*, api_key: str, timeout: float) -> Any
    create(client, *, model, max_tokens, temperature, system, tools, messages) -> ModelResponse

`messages` arrives canonical (see `normalise` in infra/provider.py) and the return value is the
neutral `ModelResponse`. Everything between those two points is that provider's dialect and stays
inside its own file.

**The import is lazy, and that is the point.** A project pins one provider SDK in
requirements.txt; importing all three at module load would make the other two a hard dependency of
starting the service. `load()` imports the one the manifest names, and an SDK that is not installed
produces a message naming the package to install rather than an ImportError from three frames down.
"""

from __future__ import annotations

import importlib
from types import ModuleType


class ProviderNotInstalled(RuntimeError):
    """The provider module is here; its SDK is not."""


# The package each provider module needs, so a missing import can say what to install instead of
# leaving somebody to work it out from a traceback.
SDK_PACKAGES = {
    "anthropic": "anthropic",
    "openai": "openai",
    "google": "google-genai",
}


def load(provider: str) -> ModuleType:
    """Import the module for one provider.

    Not cached: `importlib.import_module` already returns the module from sys.modules after the
    first call, and a second cache in front of that is a layer with nothing to do.
    """
    try:
        return importlib.import_module(f".{provider}", __name__)
    except ModuleNotFoundError as err:
        package = SDK_PACKAGES.get(provider, provider)
        # Two different failures reach here and they need different answers: the provider module
        # itself is missing (a broken checkout), or its SDK is (the ordinary case — the project
        # pins one provider and you switched).
        if err.name and err.name.startswith(__name__):
            raise
        raise ProviderNotInstalled(
            f"model.provider is {provider!r} and its SDK is not installed.\n"
            f"  pip install {package}\n"
            f"  then pin it in app/requirements.txt — an unpinned dependency changes behaviour\n"
            f"  between two deploys of the same commit (control.ai.supply.model_pinned)"
        ) from err
