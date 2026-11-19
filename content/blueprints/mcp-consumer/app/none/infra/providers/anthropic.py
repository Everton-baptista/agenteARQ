"""Anthropic, through the Messages API.

The shortest of the three, because the neutral shape in infra/provider.py was modelled on this one:
content blocks in, content blocks out, tools declared with `input_schema`. That is a bias worth
naming rather than hiding — it means the OpenAI and Google modules do the translating and this one
mostly forwards.

Note on the file name: this module is `app.infra.providers.anthropic`, and `from anthropic import
Anthropic` below resolves to the installed SDK, not to itself. Python 3 imports are absolute.
"""

from __future__ import annotations

from typing import Any

from anthropic import Anthropic

from ..provider import ModelResponse, TextBlock, ToolUseBlock, Usage


def client(*, api_key: str, timeout: float) -> Anthropic:
    return Anthropic(
        api_key=api_key,
        timeout=timeout,
        # Retries are handled by call_with_retry, which shares a circuit breaker with everything
        # else. Two independent retry layers multiply into nine attempts nobody asked for.
        max_retries=0,
    )


def create(
    sdk: Anthropic,
    *,
    model: str,
    max_tokens: int,
    temperature: float,
    system: str,
    tools: list[dict],
    messages: list[dict],
) -> ModelResponse:
    response = sdk.messages.create(
        model=model,
        max_tokens=max_tokens,
        temperature=temperature,
        system=system,
        tools=[_tool(t) for t in tools],
        messages=messages,
    )
    return ModelResponse(
        content=[b for b in map(_block, response.content) if b is not None],
        usage=Usage(
            input_tokens=response.usage.input_tokens,
            output_tokens=response.usage.output_tokens,
        ),
    )


def _tool(spec: dict) -> dict:
    return {
        "name": spec["name"],
        "description": spec["description"],
        "input_schema": spec["input_schema"],
    }


def _block(block: Any):
    """One response block, or None for a kind the agent core does not act on.

    Dropping the unknown rather than passing it through is deliberate: the runner's contract is
    "text or tool_use", and a block it has no branch for would otherwise reach `_serialisable` and
    end up in the pending-approval store as something nobody can resume from.
    """
    if block.type == "text":
        return TextBlock(text=block.text)
    if block.type == "tool_use":
        return ToolUseBlock(id=block.id, name=block.name, input=dict(block.input or {}))
    return None
