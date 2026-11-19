# Adapter: Vercel AI SDK

TypeScript-first. Zod schemas make the tool contract and the structured-output control line up naturally.

Nothing here changes what the standard requires — only where it attaches. Read
[`none-raw-sdk.md`](none-raw-sdk.md) first if you want the shape without a framework in the way.

## 1. The versioned system prompt

```ts
const { system, sha256 } = await verifiedPrompt(manifest);   // throws on mismatch
const result = await generateText({
  model: openai(manifest.model.id),
  system,
  messages,
  maxSteps: manifest.autonomy.max_steps,
});
```

## 2. Tools and the permission check

```ts
const searchOrders = tool({
  description: spec.description_for_model,
  parameters: z.object({ orderReference: z.string().regex(/^[A-Z0-9-]{6,20}$/) }),
  execute: async ({ orderReference }, { abortSignal }) => {
    // customerId comes from the request session, closed over — not a parameter.
    return ordersApi.search(session.customerId, orderReference);
  },
});
```

## 3. The three guardrail points

`experimental_prepareStep` and the `onStepFinish` callback give the action point. Input and
output checks wrap `generateText`. Use `experimental_output` with a Zod schema for format
conformance.

## 4. Telemetry

`experimental_telemetry: { isEnabled: true }` emits OTel spans. Pin the semconv version and
leave `recordInputs`/`recordOutputs` off — those are `capture_content` under another name.

## 5. Handoff and approval

No native handoff. Compose explicitly: validate the payload with Zod at the boundary, and give
the next call its own tool list rather than passing the current one through.

---

Controls this adapter materialises: `control.ai.genai.prompt_versioned`,
`control.ai.genai.untrusted_content_isolation`, `control.ai.tool.least_privilege`,
`control.ai.tool.action_guardrail`, `control.ai.tool.irreversible_requires_approval`,
`control.ai.agent.stop_conditions`, `control.ai.obs.semconv_pinned`,
`control.ai.privacy.capture_content_default_off`.
