"""AWS Bedrock provider implementation using Bearer Token / HTTP Converse API."""

from __future__ import annotations

import json
import os
import urllib.request
from typing import Any


def client(api_key: str, timeout: float = 30.0) -> dict:
    """Returns connection configuration for Bedrock HTTP Converse API."""
    return {"token": api_key, "timeout": timeout}


def create(
    client_info: dict,
    *,
    model: str,
    max_tokens: int,
    temperature: float,
    system: str,
    tools: list[dict],
    messages: list[dict],
) -> Any:
    """Execute model call using Amazon Bedrock HTTP Converse API."""
    # Resolve default Bedrock model ID if a generic model name is passed
    model_id = model
    if model in ("amazon.nova-micro", "bedrock", "claude-sonnet-4-5-20250929", "gemini-3.6-flash", "gpt-5.6-terra"):
        model_id = "amazon.nova-micro-v1:0"

    region = os.getenv("AWS_REGION", "us-east-1")
    url = f"https://bedrock-runtime.{region}.amazonaws.com/model/{model_id}/converse"

    # Format system prompt
    system_prompts = [{"text": system}] if system else []

    # Format messages for Bedrock Converse API
    bedrock_messages = []
    for msg in messages:
        role = msg["role"]
        if role == "system":
            continue
        content_blocks = []
        for block in msg.get("content", []):
            if isinstance(block, str):
                content_blocks.append({"text": block})
            elif isinstance(block, dict):
                b_type = block.get("type")
                if b_type == "text":
                    content_blocks.append({"text": block.get("text", "")})
                elif b_type == "tool_use":
                    content_blocks.append(
                        {
                            "toolUse": {
                                "toolUseId": block.get("id"),
                                "name": block.get("name"),
                                "input": block.get("input", {}),
                            }
                        }
                    )
                elif b_type == "tool_result":
                    content_blocks.append(
                        {
                            "toolResult": {
                                "toolUseId": block.get("tool_use_id"),
                                "content": [{"text": str(block.get("content", ""))}],
                            }
                        }
                    )
        if content_blocks:
            bedrock_messages.append({"role": role, "content": content_blocks})

    payload: dict[str, Any] = {
        "messages": bedrock_messages,
        "inferenceConfig": {
            "maxTokens": max_tokens,
            "temperature": temperature,
        },
    }
    if system_prompts:
        payload["system"] = system_prompts

    # Format tools if provided
    if tools:
        tool_specs = []
        for tool in tools:
            spec = tool.get("function", tool)
            tool_specs.append(
                {
                    "toolSpec": {
                        "name": spec["name"],
                        "description": spec.get("description", ""),
                        "inputSchema": {
                            "json": spec.get("parameters", spec.get("input_schema", {}))
                        },
                    }
                }
            )
        payload["toolConfig"] = {"tools": tool_specs}

    data_bytes = json.dumps(payload).encode("utf-8")
    headers = {
        "Authorization": f"Bearer {client_info['token']}",
        "Content-Type": "application/json",
    }

    req = urllib.request.Request(url, data=data_bytes, headers=headers, method="POST")
    with urllib.request.urlopen(req, timeout=client_info.get("timeout", 30.0)) as resp:
        res = json.loads(resp.read().decode("utf-8"))

    # Import response shapes from parent provider module
    from ..provider import ModelResponse, TextBlock, ToolUseBlock, Usage

    out_blocks = []
    msg_out = res.get("output", {}).get("message", {})
    for content_item in msg_out.get("content", []):
        if "text" in content_item:
            out_blocks.append(TextBlock(text=content_item["text"]))
        elif "toolUse" in content_item:
            tu = content_item["toolUse"]
            out_blocks.append(
                ToolUseBlock(
                    id=tu["toolUseId"],
                    name=tu["name"],
                    input=tu.get("input", {}),
                )
            )

    usage_raw = res.get("usage", {})
    usage = Usage(
        input_tokens=usage_raw.get("inputTokens", 0),
        output_tokens=usage_raw.get("outputTokens", 0),
    )
    return ModelResponse(content=out_blocks, usage=usage)
