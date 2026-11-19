"""The provider seam, exercised without a network call or a credential.

Every test here hands a fake client to `create()` and reads what came back. That is the whole
reason `create()` takes the client as a parameter instead of fetching it: a test that needs an API
key is a test that stops running the first time somebody clones the repository, and the rule it was
covering stops being checked at the same moment.

What is being pinned down is the pair of things that actually differ between providers — how a tool
is declared, and how a response is read — plus the one that bites in practice: a tool result is a
block inside a user turn for Anthropic, a message of its own for OpenAI, and keyed by name rather
than by id for Google.

The Anthropic tests always run. The other two skip unless that SDK is installed, because a project
pins exactly one provider SDK (see requirements.txt). This repository's own CI installs all three,
so the translations are covered here even though a generated project only installs one.
"""

from __future__ import annotations

import json
from types import SimpleNamespace

import pytest

from app.infra import provider

TOOLS = [
    {
        "name": "lookup_order",
        "description": "Look up one order.",
        "input_schema": {
            "type": "object",
            "properties": {"order_id": {"type": "string", "description": "The id."}},
            "required": ["order_id"],
            "additionalProperties": False,
        },
    }
]

# One full turn: a question, an assistant turn that called a tool, and the result answering it.
HISTORY = [
    {"role": "user", "content": "where is BR-77120?"},
    {
        "role": "assistant",
        "content": [
            provider.TextBlock(text="Let me look."),
            provider.ToolUseBlock(id="call-1", name="lookup_order", input={"order_id": "BR-77120"}),
        ],
    },
    {
        "role": "user",
        "content": [
            {
                "type": "tool_result",
                "tool_use_id": "call-1",
                "content": json.dumps({"status": "in transit"}),
                "is_error": False,
            }
        ],
    },
]


# ---------------------------------------------------------------- the neutral layer


def test_normalise_flattens_strings_objects_and_dicts():
    """A resumed run carries dicts where a fresh one carries objects. Both have to survive."""
    canonical = provider.normalise(HISTORY)

    assert canonical[0]["content"] == [{"type": "text", "text": "where is BR-77120?"}]
    assert canonical[1]["content"][1] == {
        "type": "tool_use",
        "id": "call-1",
        "name": "lookup_order",
        "input": {"order_id": "BR-77120"},
    }
    # Normalising twice is normalising once. The approval path serialises a paused run and hands it
    # back, so this is the round trip that actually happens rather than a hypothetical one.
    assert provider.normalise(canonical) == canonical


def test_unknown_provider_is_refused_rather_than_defaulted():
    with pytest.raises(provider.UnknownProvider):
        provider.credential_name("acme-llm")
    # And the readiness probe reports it as absent rather than raising out of a health endpoint.
    assert provider.credential_present("acme-llm") is False


def test_every_provider_has_a_credential_name():
    """A provider in PROVIDERS with no name would fail only at the first call, in production."""
    for name in provider.PROVIDERS:
        assert provider.credential_name(name).endswith("_API_KEY")


# ---------------------------------------------------------------- anthropic


def _anthropic_client(captured: dict):
    def create(**kwargs):
        captured.update(kwargs)
        return SimpleNamespace(
            content=[
                SimpleNamespace(type="text", text="It is in transit."),
                SimpleNamespace(type="redacted_thinking", data="opaque"),
            ],
            usage=SimpleNamespace(input_tokens=11, output_tokens=7),
        )

    return SimpleNamespace(messages=SimpleNamespace(create=create))


def test_anthropic_declares_tools_with_input_schema_and_reads_blocks():
    from app.infra.providers import anthropic as impl

    captured: dict = {}
    result = impl.create(
        _anthropic_client(captured),
        model="m",
        max_tokens=100,
        temperature=0.2,
        system="be careful",
        tools=TOOLS,
        messages=provider.normalise(HISTORY),
    )

    assert captured["tools"][0]["input_schema"] == TOOLS[0]["input_schema"]
    assert captured["system"] == "be careful"
    # A block kind the runner has no branch for is dropped rather than passed through: it would
    # otherwise reach the pending-approval store as something nothing can resume from.
    assert [b.type for b in result.content] == ["text"]
    assert (result.usage.input_tokens, result.usage.output_tokens) == (11, 7)


# ---------------------------------------------------------------- openai


def _openai_client(captured: dict):
    def create(**kwargs):
        captured.update(kwargs)
        return SimpleNamespace(
            choices=[
                SimpleNamespace(
                    message=SimpleNamespace(
                        content="It is in transit.",
                        tool_calls=[
                            SimpleNamespace(
                                id="call-2",
                                function=SimpleNamespace(
                                    name="lookup_order", arguments='{"order_id": "BR-9"}'
                                ),
                            )
                        ],
                    )
                )
            ],
            usage=SimpleNamespace(prompt_tokens=11, completion_tokens=7),
        )

    return SimpleNamespace(chat=SimpleNamespace(completions=SimpleNamespace(create=create)))


def test_openai_moves_the_system_prompt_and_splits_tool_results_into_messages():
    pytest.importorskip("openai")
    from app.infra.providers import openai as impl

    captured: dict = {}
    result = impl.create(
        _openai_client(captured),
        model="m",
        max_tokens=100,
        temperature=0.2,
        system="be careful",
        tools=TOOLS,
        messages=provider.normalise(HISTORY),
    )

    sent = captured["messages"]
    assert sent[0] == {"role": "system", "content": "be careful"}
    assert [m["role"] for m in sent] == ["system", "user", "assistant", "tool"]
    # The tool schema moves inside `function`, under `parameters`.
    assert captured["tools"][0]["function"]["parameters"] == TOOLS[0]["input_schema"]
    # Arguments go over the wire as a JSON string, not as an object.
    assert json.loads(sent[2]["tool_calls"][0]["function"]["arguments"]) == {"order_id": "BR-77120"}
    assert sent[3]["tool_call_id"] == "call-1"
    # `max_tokens` is rejected on the reasoning models; the parameter that works everywhere is the
    # one this seam sends.
    assert "max_tokens" not in captured and captured["max_completion_tokens"] == 100

    assert [b.type for b in result.content] == ["text", "tool_use"]
    assert result.content[1].input == {"order_id": "BR-9"}
    assert (result.usage.input_tokens, result.usage.output_tokens) == (11, 7)


def test_openai_unparseable_arguments_become_an_empty_call_not_a_crash():
    """The tool's own input validation is where a bad argument is supposed to be refused.

    Refusing there produces a tool_result the model can read and correct; raising here ends the run
    with a traceback the model never sees.
    """
    pytest.importorskip("openai")
    from app.infra.providers import openai as impl

    assert impl._arguments("{not json") == {}
    assert impl._arguments("[1, 2]") == {}
    assert impl._arguments(None) == {}


# ---------------------------------------------------------------- google


def _google_client(captured: dict):
    from google.genai import types

    def generate_content(**kwargs):
        captured.update(kwargs)
        return SimpleNamespace(
            candidates=[
                SimpleNamespace(
                    content=SimpleNamespace(
                        parts=[
                            types.Part(text="It is in transit."),
                            types.Part(
                                function_call=types.FunctionCall(
                                    name="lookup_order", args={"order_id": "BR-9"}
                                )
                            ),
                        ]
                    )
                )
            ],
            usage_metadata=SimpleNamespace(prompt_token_count=11, candidates_token_count=7),
        )

    return SimpleNamespace(models=SimpleNamespace(generate_content=generate_content))


def test_google_declares_function_declarations_and_keys_results_by_name():
    pytest.importorskip("google.genai")
    from app.infra.providers import google as impl

    captured: dict = {}
    result = impl.create(
        _google_client(captured),
        model="m",
        max_tokens=100,
        temperature=0.2,
        system="be careful",
        tools=TOOLS,
        messages=provider.normalise(HISTORY),
    )

    config = captured["config"]
    assert config.system_instruction == "be careful"
    assert config.tools[0].function_declarations[0].name == "lookup_order"
    # Automatic function calling would execute callables inside the SDK, below every guardrail.
    assert config.automatic_function_calling.disable is True

    contents = captured["contents"]
    assert [c.role for c in contents] == ["user", "model", "user"]
    # The result is bound to the call by name, which is the difference that bites.
    response_part = contents[2].parts[0]
    assert response_part.function_response.name == "lookup_order"

    assert [b.type for b in result.content] == ["text", "tool_use"]
    # The API may return no id; the seam supplies one so the result can still be matched after a
    # paused run has been serialised and reloaded.
    assert result.content[1].id.startswith("lookup_order")
    assert (result.usage.input_tokens, result.usage.output_tokens) == (11, 7)


def test_google_schema_drops_keywords_gemini_rejects():
    pytest.importorskip("google.genai")
    from app.infra.providers import google as impl

    cleaned = impl._schema(TOOLS[0]["input_schema"])
    assert "additionalProperties" not in cleaned
    assert cleaned["properties"]["order_id"] == {"type": "string", "description": "The id."}
    assert cleaned["required"] == ["order_id"]


def test_google_drops_a_result_whose_call_it_never_saw():
    """Naming a guess would attach this output to a different tool call, and the model would act
    on it. Sending nothing is recoverable; sending the wrong answer is not."""
    pytest.importorskip("google.genai")
    from app.infra.providers import google as impl

    orphan = provider.normalise(
        [{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "ghost", "content": "x"}]}]
    )
    assert impl._contents(orphan) == []
