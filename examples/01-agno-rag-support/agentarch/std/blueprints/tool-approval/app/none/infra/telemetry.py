"""OpenTelemetry spans and cost metrics — the code half of what the manifest declares.

`observability.otel.enabled: true` in a manifest is a claim. Until something emits a span it is a
claim about nothing, and `control.ai.obs.otel_enabled` is satisfied by a YAML field while the
service is unobservable. This module is where the claim becomes true.

Three things it gets right, each of which is a rule from standard 12:

  the semconv version is pinned      — attribute names change between versions of the GenAI
                                       conventions, and a dashboard built on renamed attributes
                                       goes blank without erroring
  content is not captured            — `capture_content: false` means the prompt, the retrieved
                                       passages and the answer never become span attributes. A
                                       trace backend is a log aggregator with better indexing, and
                                       putting a customer's question in one is the same mistake as
                                       putting it in an access log
  the agent span is a child          — of the request span, so an agent failure is visible in the
                                       same trace as the request that caused it

It degrades to no-ops when the OTel packages are absent, so the blueprint runs with a smaller
dependency set and the tracing is a deployment decision rather than a prerequisite for reading the
code.
"""

from __future__ import annotations

import contextlib
import os
import time
from typing import Any, Iterator

# The pinned version. It appears here and in the manifest, and control.ai.obs.semconv_pinned
# checks the manifest declares one — this constant is what the code actually emits under.
SEMCONV_VERSION = "1.29.0"

# Read once. A per-request env lookup to decide whether to record content is a per-request chance
# of getting it wrong.
CAPTURE_CONTENT = os.getenv("OTEL_CAPTURE_CONTENT", "false").lower() == "true"

try:  # pragma: no cover - exercised by whether the extra is installed
    from opentelemetry import metrics, trace

    _tracer = trace.get_tracer("agentarch.tool-approval", SEMCONV_VERSION)
    _meter = metrics.get_meter("agentarch.tool-approval", SEMCONV_VERSION)
    _cost = _meter.create_counter(
        "gen_ai.client.cost", unit="USD", description="Estimated spend, by model and tenant."
    )
    _tokens = _meter.create_counter(
        "gen_ai.client.token.usage", unit="token", description="Tokens in and out, by type."
    )
    _guardrail = _meter.create_counter(
        "agent.guardrail.decisions", description="Guardrail outcomes, by point and decision."
    )
    ENABLED = True
except ImportError:  # pragma: no cover
    ENABLED = False
    _tracer = _meter = _cost = _tokens = _guardrail = None


# Per million tokens. Wrong the day a price changes, which is why the number lives in one place and
# the metric records the model id alongside the cost — a dashboard can then be corrected after the
# fact instead of silently reporting the old price forever.
#
# One row per model this project can be configured to call, not only the one it is configured to
# call today. A model with no row costs zero, and a cost of zero makes `autonomy.budget.usd_per_run`
# a limit that can never be reached — the budget would still be declared, still be checked, and
# never fire. Switching provider must not quietly disable the spend cap.
#
# Reviewed 2026-07-31.
PRICE_PER_MTOK = {
    "claude-sonnet-4-5-20250929": {"input": 3.00, "output": 15.00},
    "gpt-5.6-terra": {"input": 2.00, "output": 12.00},
    "gemini-3.6-flash": {"input": 1.50, "output": 7.50},
}


@contextlib.contextmanager
def span(name: str, **attributes: Any) -> Iterator[Any]:
    """Start a span, or do nothing at all.

    Attributes passed here are metadata: identifiers, counts, decisions, model names. Never
    content. There is no keyword argument for the prompt because there should be no temptation.
    """
    if not ENABLED:
        yield _NullSpan()
        return
    with _tracer.start_as_current_span(name) as current:
        for key, value in attributes.items():
            if value is not None:
                current.set_attribute(key, value)
        try:
            yield current
        except Exception as err:
            current.record_exception(err)
            current.set_status(trace.Status(trace.StatusCode.ERROR, type(err).__name__))
            raise


def record_model_call(
    *, model: str, tenant: str, input_tokens: int, output_tokens: int, latency_ms: float
) -> float:
    """Attribute one provider call to a model and a tenant, and return its estimated cost.

    Returning the number as well as recording it is deliberate: the caller can enforce a per-run
    budget with the same figure that appears on the dashboard, so the bill and the limit cannot
    disagree.
    """
    price = PRICE_PER_MTOK.get(model, {"input": 0.0, "output": 0.0})
    cost = (input_tokens * price["input"] + output_tokens * price["output"]) / 1_000_000

    if not ENABLED:
        return cost

    common = {"gen_ai.request.model": model, "tenant": tenant}
    _tokens.add(input_tokens, {**common, "gen_ai.token.type": "input"})
    _tokens.add(output_tokens, {**common, "gen_ai.token.type": "output"})
    _cost.add(cost, common)

    current = trace.get_current_span()
    current.set_attribute("gen_ai.usage.input_tokens", input_tokens)
    current.set_attribute("gen_ai.usage.output_tokens", output_tokens)
    current.set_attribute("gen_ai.client.operation.duration", latency_ms / 1000)
    current.set_attribute("gen_ai.client.cost", cost)
    return cost


def record_guardrail(point: str, decision: str, control: str = "") -> None:
    """Count a guardrail outcome.

    Worth its own metric because "how often does the input guardrail refuse" is the question that
    tells you whether it is calibrated, and it is unanswerable from traces alone once you are
    sampling. The reason for a refusal is a category, never the text that triggered it.
    """
    if not ENABLED:
        return
    _guardrail.add(1, {"point": point, "decision": decision, "control": control})
    trace.get_current_span().set_attribute(f"agent.guardrail.{point}", decision)


def configure(service_name: str, service_version: str) -> bool:
    """Wire up an exporter if one is configured, and say whether tracing is live.

    Reads the standard OTEL_* environment variables rather than inventing its own, so this works
    with whatever collector your platform already runs. With none set, it stays off — a service
    that fails to start because no collector is reachable is worse than one that is not traced.
    """
    if not ENABLED or not os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
        return False

    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor

    resource = Resource.create(
        {
            "service.name": service_name,
            "service.version": service_version,
            # Recorded so a trace can be read against the conventions it was produced under.
            "telemetry.semconv.version": SEMCONV_VERSION,
        }
    )
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
    trace.set_tracer_provider(provider)
    return True


class _NullSpan:
    """Stands in when OTel is not installed, so call sites need no conditionals."""

    def set_attribute(self, *_: Any, **__: Any) -> None:
        return None

    def record_exception(self, *_: Any, **__: Any) -> None:
        return None

    def set_status(self, *_: Any, **__: Any) -> None:
        return None


class Timer:
    """Wall-clock milliseconds, for the latency attributes above."""

    def __enter__(self) -> "Timer":
        self._start = time.perf_counter()
        return self

    def __exit__(self, *_: Any) -> None:
        self.ms = (time.perf_counter() - self._start) * 1000

    ms: float = 0.0
