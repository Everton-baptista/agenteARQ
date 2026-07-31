"""OpenAI, through Chat Completions.

Three translations, and they are the whole file:

  - the system prompt is a message with `role: "system"`, not a top-level field
  - a tool is `{"type": "function", "function": {..., "parameters": <schema>}}`
  - a tool result is its own message with `role: "tool"`, keyed by `tool_call_id`

The last one is the shape difference that matters. Anthropic carries tool results as blocks inside
a user turn, so several results share one message; OpenAI wants one message per result. A run that
calls two tools in one step produces two messages here and one there, and getting that wrong
produces an error from the API rather than a wrong answer — which is the good kind of wrong.

`max_completion_tokens` rather than `max_tokens`: the latter is rejected on the reasoning models
and deprecated on the rest, so the parameter that works everywhere is the one to write.
"""

from __future__ import annotations

import json
from typing import Any

from openai import OpenAI

from ..provider import ModelResponse, TextBlock, ToolUseBlock, Usage, tool_result_text


def client(*, api_key: str, timeout: float) -> OpenAI:
    return OpenAI(
        api_key=api_key,
        timeout=timeout,
        # Retries are handled by call_with_retry, which shares a circuit breaker with everything
        # else. Two independent retry layers multiply into nine attempts nobody asked for.
        max_retries=0,
    )


def create(
    sdk: OpenAI,
    *,
    model: str,
    max_tokens: int,
    temperature: float,
    system: str,
    tools: list[dict],
    messages: list[dict],
) -> ModelResponse:
    request: dict[str, Any] = {
        "model": model,
        "max_completion_tokens": max_tokens,
        "temperature": temperature,
        "messages": [{"role": "system", "content": system}] + _messages(messages),
    }
    if tools:
        request["tools"] = [_tool(t) for t in tools]

    response = sdk.chat.completions.create(**request)
    choice = response.choices[0].message

    content: list = []
    if choice.content:
        content.append(TextBlock(text=choice.content))
    for call in choice.tool_calls or []:
        content.append(
            ToolUseBlock(
                id=call.id,
                name=call.function.name,
                input=_arguments(call.function.arguments),
            )
        )

    usage = response.usage
    return ModelResponse(
        content=content,
        usage=Usage(
            input_tokens=getattr(usage, "prompt_tokens", 0) or 0,
            output_tokens=getattr(usage, "completion_tokens", 0) or 0,
        ),
    )


def _tool(spec: dict) -> dict:
    return {
        "type": "function",
        "function": {
            "name": spec["name"],
            "description": spec["description"],
            "parameters": spec["input_schema"],
        },
    }


def _messages(canonical: list[dict]) -> list[dict]:
    out: list[dict] = []
    for message in canonical:
        role = message["role"]
        text = "".join(b.get("text", "") for b in message["content"] if b.get("type") == "text")
        uses = [b for b in message["content"] if b.get("type") == "tool_use"]
        results = [b for b in message["content"] if b.get("type") == "tool_result"]

        # Results first: they answer the assistant turn immediately before, and OpenAI validates
        # that ordering. One message each — see the note at the top of the file.
        for result in results:
            out.append(
                {
                    "role": "tool",
                    "tool_call_id": result.get("tool_use_id", ""),
                    "content": tool_result_text(result),
                }
            )

        if role == "assistant":
            if not text and not uses:
                continue
            entry: dict[str, Any] = {"role": "assistant", "content": text or None}
            if uses:
                entry["tool_calls"] = [
                    {
                        "id": u.get("id", ""),
                        "type": "function",
                        "function": {
                            "name": u.get("name", ""),
                            "arguments": json.dumps(u.get("input", {})),
                        },
                    }
                    for u in uses
                ]
            out.append(entry)
            continue

        if text:
            out.append({"role": "user", "content": text})
    return out


def _arguments(raw: str | None) -> dict:
    """Tool arguments arrive as a JSON string, and a model can emit one that does not parse.

    Returning `{}` rather than raising hands the empty argument set to the tool's own input
    validation, which is where a bad argument is supposed to be refused — and refusing there
    produces a tool_result the model can read and correct, instead of a traceback that ends the run.
    """
    if not raw:
        return {}
    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError:
        return {}
    return parsed if isinstance(parsed, dict) else {}
