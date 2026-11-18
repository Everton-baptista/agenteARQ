"""Structured metadata logs without interpolating arbitrary SDK messages or exception bodies."""
import json
import logging
import os

_FIELDS = ("request_id", "method", "route", "status", "duration_ms", "tenant", "subject",
           "tool", "decision", "action_hash", "reason_hash")


class MetadataFormatter(logging.Formatter):
    def format(self, record):
        event = record.getMessage() if record.name in ("api", "api.access", "agent.approval.audit") else record.name
        payload = {"level": record.levelname, "event": event}
        payload.update({key: getattr(record, key) for key in _FIELDS if hasattr(record, key)})
        if record.exc_info and record.exc_info[0]:
            payload["exception_type"] = record.exc_info[0].__name__
        return json.dumps(payload, ensure_ascii=True, default=str)


def configure():
    handler = logging.StreamHandler()
    handler.setFormatter(MetadataFormatter())
    root = logging.getLogger()
    # Only replace our own handler; a test runner or embedding application owns its handlers.
    for current in list(root.handlers):
        if getattr(current, "_agentarch_metadata", False):
            root.removeHandler(current)
    handler._agentarch_metadata = True
    root.addHandler(handler)
    root.setLevel(os.getenv("LOG_LEVEL", "INFO"))
    logging.getLogger("uvicorn.access").disabled = True
