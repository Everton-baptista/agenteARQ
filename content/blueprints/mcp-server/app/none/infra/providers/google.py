"""Google, through the Gemini API (`google-genai`).

The furthest from the neutral shape, in three ways:

  - a turn is a `Content` with `parts`, and the assistant's role is spelled `model`, not `assistant`
  - tools are `function_declarations` inside a `Tool`
  - **a function response is keyed by name, not by call id**

That last one is the trap. Every other provider correlates a result to a call by an opaque id; here
the id is optional and the name is what binds them. So the history walk keeps an id → name map as it
goes, and a tool result whose id was never seen is dropped rather than sent with a guessed name —
answering the wrong call is worse than answering none.

Automatic function calling is disabled explicitly. The SDK will otherwise execute Python callables
it finds in `tools`, which would put tool execution inside the provider library, below every
guardrail in app/agent. There are no callables here, so it is a no-op today and a guard against the
day somebody adds one.
"""

from __future__ import annotations

from typing import Any

from google import genai
from google.genai import types

from ..provider import ModelResponse, TextBlock, ToolUseBlock, Usage, tool_result_text

# What Gemini's Schema accepts. JSON Schema keywords outside this set are rejected by the API, and
# `additionalProperties` — which the tool specs carry, correctly, for the other two providers — is
# the one that trips it in practice.
SCHEMA_KEYS = frozenset(
    {
        "type",
        "format",
        "title",
        "description",
        "nullable",
        "enum",
        "items",
        "properties",
        "required",
        "minimum",
        "maximum",
        "min_items",
        "max_items",
    }
)


def client(*, api_key: str, timeout: float) -> genai.Client:
    return genai.Client(
        api_key=api_key,
        # Milliseconds here, seconds everywhere else in this project. The conversion is explicit
        # rather than a constant, because a timeout that is a thousand times too short fails every
        # call and a timeout a thousand times too long fails none of them until production.
        http_options=types.HttpOptions(timeout=int(timeout * 1000)),
    )


def create(
    sdk: genai.Client,
    *,
    model: str,
    max_tokens: int,
    temperature: float,
    system: str,
    tools: list[dict],
    messages: list[dict],
) -> ModelResponse:
    config = types.GenerateContentConfig(
        system_instruction=system,
        max_output_tokens=max_tokens,
        temperature=temperature,
        automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
    )
    if tools:
        config.tools = [types.Tool(function_declarations=[_declaration(t) for t in tools])]

    response = sdk.models.generate_content(
        model=model, contents=_contents(messages), config=config
    )

    content: list = []
    candidates = response.candidates or []
    parts = candidates[0].content.parts if candidates and candidates[0].content else []
    for index, part in enumerate(parts or []):
        if getattr(part, "text", None):
            content.append(TextBlock(text=part.text))
            continue
        call = getattr(part, "function_call", None)
        if call is not None:
            content.append(
                ToolUseBlock(
                    id=call.id or _synthetic_id(call.name, index),
                    name=call.name,
                    input=dict(call.args or {}),
                )
            )

    usage = response.usage_metadata
    return ModelResponse(
        content=content,
        usage=Usage(
            input_tokens=getattr(usage, "prompt_token_count", 0) or 0,
            output_tokens=getattr(usage, "candidates_token_count", 0) or 0,
        ),
    )


def _synthetic_id(name: str, index: int) -> str:
    """An id for a call the API did not give one.

    The name is in it so that a result can still be matched after a paused run is serialised and
    reloaded, when the in-memory map from this process is gone.
    """
    return f"{name}#{index}"


def _declaration(spec: dict) -> dict:
    return {
        "name": spec["name"],
        "description": spec["description"],
        "parameters": _schema(spec["input_schema"]),
    }


def _schema(node: Any) -> Any:
    """The same JSON Schema, minus the keywords Gemini rejects.

    Filtering rather than rewriting: a keyword this function drops is a constraint the model no
    longer sees, and the tool's own input validation is what still enforces it. That asymmetry is
    the reason validation belongs in the tool and not in the schema.
    """
    if isinstance(node, list):
        return [_schema(item) for item in node]
    if not isinstance(node, dict):
        return node
    out = {}
    for key, value in node.items():
        if key not in SCHEMA_KEYS:
            continue
        if key == "properties" and isinstance(value, dict):
            out[key] = {k: _schema(v) for k, v in value.items()}
            continue
        out[key] = _schema(value)
    return out


def _contents(canonical: list[dict]) -> list[types.Content]:
    out: list[types.Content] = []
    names: dict[str, str] = {}  # tool_use id → tool name, for the result that answers it

    for message in canonical:
        role = "model" if message["role"] == "assistant" else "user"
        parts: list[types.Part] = []

        for block in message["content"]:
            kind = block.get("type")
            if kind == "text" and block.get("text"):
                parts.append(types.Part(text=block["text"]))
            elif kind == "tool_use":
                names[block.get("id", "")] = block.get("name", "")
                parts.append(
                    types.Part(
                        function_call=types.FunctionCall(
                            id=block.get("id"),
                            name=block.get("name", ""),
                            args=block.get("input", {}),
                        )
                    )
                )
            elif kind == "tool_result":
                name = names.get(block.get("tool_use_id", ""))
                if not name:
                    # No call to answer. See the note at the top: naming a guess would attach this
                    # output to a different tool call, and the model would act on it.
                    continue
                parts.append(
                    types.Part(
                        function_response=types.FunctionResponse(
                            id=block.get("tool_use_id"),
                            name=name,
                            response={
                                "output": tool_result_text(block),
                                "is_error": bool(block.get("is_error")),
                            },
                        )
                    )
                )

        if parts:
            out.append(types.Content(role=role, parts=parts))
    return out
