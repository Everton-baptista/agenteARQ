# Adapter: Semantic Kernel

Filters are the extension point and map onto the three points directly. Plugins are the tool boundary.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

Prompt templates live in files already, which fits `prompts.system.path` naturally. Verify the
hash when the kernel loads the template rather than at first use.

## 2. Tools and the permission check

Plugins and `KernelFunction`. Read `effect` from the `.tool.yaml` in the invocation filter.

## 3. The three guardrail points

```csharp
kernel.PromptRenderFilters.Add(new InputGuardrailFilter());       // input
kernel.FunctionInvocationFilters.Add(new ActionGuardrailFilter()); // action — the important one
kernel.AutoFunctionInvocationFilters.Add(new OutputGuardrailFilter());
```

A filter that does not call `next` is the deny path.

## 4. Telemetry

Semantic Kernel emits OTel natively. Pin the semconv version and turn off content capture; the
SDK's default is tuned for debugging, not for a privacy assessment.

## 5. Handoff and approval

Agent framework handoffs and `AgentGroupChat`. Termination strategy is `autonomy.stop_conditions`
— set it explicitly rather than relying on the default maximum, which is a limit nobody chose.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
