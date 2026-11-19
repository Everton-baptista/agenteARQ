"""Deterministic tool contracts and single-operation approval capabilities."""
from __future__ import annotations

import hashlib
import json
from dataclasses import asdict, dataclass

from jsonschema import Draft202012Validator
from jsonschema.exceptions import SchemaError, ValidationError


def digest(value: object) -> str:
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), default=str).encode()).hexdigest()


def validate_arguments(args: dict, spec: dict) -> tuple[bool, str]:
    schema = spec.get("input_schema")
    if not isinstance(schema, dict):
        return False, "tool input_schema is missing"
    def local_refs(node):
        if isinstance(node, dict):
            if "$ref" in node and not str(node["$ref"]).startswith("#"):
                raise ValueError("external schema references are not permitted")
            for value in node.values(): local_refs(value)
        elif isinstance(node, list):
            for value in node: local_refs(value)
    try:
        local_refs(schema)
        Draft202012Validator.check_schema(schema)
        Draft202012Validator(schema).validate(args)
    except (SchemaError, ValidationError, ValueError) as err:
        return False, "tool arguments do not satisfy the reviewed input_schema: " + type(err).__name__
    return True, ""


@dataclass
class ApprovalGrant:
    """Created by the trusted approval boundary, never from request/model fields.

    Bound to one provider call, its complete arguments, reviewed tool contract and original
    requester. A grant consumes itself before dispatch, including when the implementation fails.
    """
    call_id: str
    tool: str
    arguments_hash: str
    spec_hash: str
    principal_hash: str
    used: bool = False

    @classmethod
    def for_call(cls, call: dict, spec: dict, principal) -> ApprovalGrant:
        return cls(call["id"], call["name"], digest(call["input"]), digest(spec), digest(asdict(principal)))

    def consume(self, call_id: str, name: str, args: dict, spec: dict, principal) -> bool:
        if self.used or (self.call_id, self.tool, self.arguments_hash, self.spec_hash, self.principal_hash) != (
            call_id, name, digest(args), digest(spec), digest(asdict(principal))
        ):
            return False
        self.used = True
        return True
