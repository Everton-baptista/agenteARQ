"""The model provider seam: one call shape, three providers behind it.

`create_message()` is the contract the agent core calls, and it does not change when the provider
does. Everything above this module — the runner, the tool dispatch, the guardrails — speaks one
vocabulary: content blocks and a token count. Everything below translates that into whichever SDK
is installed.

Two things differ between providers, and almost nothing else does:

  - the tool schema      `input_schema` · `parameters` inside `function` · `function_declarations`
  - the response shape   `content` blocks · `tool_calls` on the message · `functionCall` in parts

Timeout, the retry boundary, and where the credential comes from are common, so they live here.
One module touches the credential, and it gets the value from infra/secrets.py rather than reading
the environment itself — so swapping to a secret manager is a change there and nothing here.

The retry boundary is here rather than in the runner for a reason worth stating: what is safe to
repeat is a property of the call, and only this layer knows that a model call which produced no tool
use can be repeated while a tool call that sent a message cannot.

**The provider is a parameter, not a global.** It arrives from `model.provider` in the manifest, via
the caller that already read the manifest. Reading it here would mean infra importing the agent
layer, which is the import direction control.ai.api.core_transport_separated exists to keep one-way
— and a provider chosen by an environment variable is a model decision that no review ever saw.
"""

from __future__ import annotations

import functools
import json
from dataclasses import dataclass, field
from typing import Any

from .providers import load
from .resilience import REQUEST_TIMEOUT_SECONDS, call_with_retry
from .secrets import SecretNotFound, resolve

# The credential name per provider. Names, never values — invariant 3. Each is also declared in
# .env.example, which is committed precisely so a new developer knows what to set without anybody
# having to send them a secret over chat.
CREDENTIAL_NAMES = {
    "anthropic": "ANTHROPIC_API_KEY",
    "openai": "OPENAI_API_KEY",
    "google": "GOOGLE_API_KEY",
}

PROVIDERS = tuple(CREDENTIAL_NAMES)


class UnknownProvider(RuntimeError):
    """The manifest names a provider this seam does not implement.

    Raised rather than quietly defaulting to one: a project whose manifest says `openai` and whose
    calls go to Anthropic is a manifest describing something other than what runs, which is the
    failure the whole standard exists to prevent.
    """


# ---------------------------------------------------------------- the neutral shapes
#
# Modelled on content blocks rather than on a chat string, because the block form is the only one
# that carries a tool call losslessly. Flattening a tool call into text and parsing it back is how
# an agent ends up calling something that was never declared.


@dataclass(frozen=True)
class TextBlock:
    text: str
    type: str = "text"

    def model_dump(self) -> dict:
        return {"type": "text", "text": self.text}


@dataclass(frozen=True)
class ToolUseBlock:
    id: str
    name: str
    input: dict = field(default_factory=dict)
    type: str = "tool_use"

    def model_dump(self) -> dict:
        return {"type": "tool_use", "id": self.id, "name": self.name, "input": self.input}


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0


@dataclass(frozen=True)
class ModelResponse:
    content: list = field(default_factory=list)
    usage: Usage = field(default_factory=Usage)


def credential_name(provider: str) -> str:
    """The environment name this provider's credential is referenced by."""
    try:
        return CREDENTIAL_NAMES[provider]
    except KeyError:
        raise UnknownProvider(
            f"{provider!r} is not a provider this project implements.\n"
            f"  the manifest field is model.provider; this seam ships {', '.join(PROVIDERS)}\n"
            "  adding one means a module under app/infra/providers/ and a line in CREDENTIAL_NAMES"
        ) from None


@functools.lru_cache(maxsize=len(CREDENTIAL_NAMES))
def model_client(provider: str) -> Any:
    """One client per provider, for the process.

    Cached because the client holds a connection pool: constructing one per request works and then
    quietly becomes the reason p99 latency is bad under load.
    """
    impl = load(provider)
    return impl.client(
        api_key=resolve(credential_name(provider)),
        # An explicit timeout, because the SDK defaults are generous enough that a hung call
        # occupies a worker long past the point where the caller has given up.
        timeout=REQUEST_TIMEOUT_SECONDS,
    )


def create_message(
    *,
    provider: str,
    model: str,
    max_tokens: int,
    temperature: float,
    system: str,
    tools: list[dict],
    messages: list[dict],
) -> ModelResponse:
    """One provider call, with retry, jitter and the breaker in front.

    Safe to repeat: the call has no side effect beyond cost, and a retried call that produces a
    tool_use has not performed it — dispatch does, and dispatch is never retried.

    `tools` and `messages` arrive in the neutral shape above. Translating them is the provider
    module's whole job, and it happens inside the retried closure so that a retry re-sends exactly
    what the first attempt did.
    """
    impl = load(provider)
    client = model_client(provider)
    canonical = normalise(messages)
    return call_with_retry(
        lambda: impl.create(
            client,
            model=model,
            max_tokens=max_tokens,
            temperature=temperature,
            system=system,
            tools=tools,
            messages=canonical,
        ),
        what=f"{provider}.create_message",
    )


def credential_present(provider: str) -> bool:
    """For the readiness probe.

    A service that starts without its credential and fails every request looks healthy to a load
    balancer, which then sends it all the traffic.
    """
    try:
        resolve(credential_name(provider))
        return True
    except (SecretNotFound, UnknownProvider):
        return False


# ---------------------------------------------------------------- the canonical history
#
# The runner appends three shapes as a run proceeds: a plain string turn, an assistant turn holding
# the blocks a response returned, and a user turn holding tool results. A run resumed after an
# approval carries those same blocks as plain dicts, because a paused run is serialised to a store.
# Both forms have to survive the round trip, so everything is flattened to dicts once, here, and the
# provider modules only ever read dicts.


def normalise(messages: list[dict]) -> list[dict]:
    """Flatten the runner's history into `{"role": ..., "content": [dict, ...]}`."""
    out = []
    for message in messages:
        content = message["content"]
        if isinstance(content, str):
            out.append({"role": message["role"], "content": [{"type": "text", "text": content}]})
            continue
        out.append({"role": message["role"], "content": [as_dict(b) for b in content]})
    return out


def as_dict(block: Any) -> dict:
    """One content block as plain data, whether it arrived as an object or already as a dict."""
    if isinstance(block, dict):
        return block
    if hasattr(block, "model_dump"):
        return block.model_dump()
    return dict(block)


def tool_result_text(block: dict) -> str:
    """The text of a tool result, however it was handed over.

    dispatch() returns a mapping and the runner JSON-encodes it, but a hand-written test may pass a
    string or a list of blocks. Every provider needs exactly one string, so the coercion happens
    once rather than three times slightly differently.
    """
    content = block.get("content", "")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(part.get("text", "") for part in content if isinstance(part, dict))
    return json.dumps(content)


# Kept as an alias so a caller that only wants to report a missing credential does not have to
# import the exception from two modules.
MissingCredential = SecretNotFound
