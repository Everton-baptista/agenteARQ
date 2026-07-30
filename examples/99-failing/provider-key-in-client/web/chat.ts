// This is the defect. A credential in a browser bundle is a public credential, and a client that
// calls a provider directly has no input guardrail, no output guardrail, no tool authorisation, no
// budget and no audit trail — every one of those lives on the server side.
import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  apiKey: import.meta.env.VITE_ANTHROPIC_API_KEY,   // served to every visitor
  dangerouslyAllowBrowser: true,
});

export async function ask(question: string) {
  // What should be here instead: fetch("/v1/ask", ...) — the client sends a question and receives
  // an answer, and every check runs where it can be enforced.
  return client.messages.create({
    model: "claude-sonnet-4-5-20250929",
    max_tokens: 1024,
    messages: [{ role: "user", content: question }],
  });
}
