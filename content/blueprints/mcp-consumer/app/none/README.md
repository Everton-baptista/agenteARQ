# Running this agent

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r app/requirements.txt
export ANTHROPIC_API_KEY=...

# Point the allowlisted server at a directory you control first.
python app/agent.py "what does the documentation say about rollbacks?"
```

Before running it at all:

```bash
agentarch mcp audit            # the allowlist on its own
agentarch mcp audit --probe    # against what the servers actually serve today
```

## The thing this blueprint exists for

A server's tool description is text the model treats as authoritative. It is fetched at connect
time and carries no version, so a server can serve a benign description while it is being
reviewed and a hostile one afterwards — a rug pull — and nothing in the protocol notices.

`tool_description_sha256` in the allowlist records what each description said when a person read
it. `AllowlistedMCPClient.tools()` compares them on every connect and refuses to start when one
has moved. Try it: change a description in the server you point at and run again.

## What else the allowlist buys you

**`default: deny`** — an allowlist that defaults to allow is a list.

**`tools_allow`** — a tool the server starts offering later is refused until someone adds it,
which is a review with a date and a name against it.

**`env_allow`** — only these variables reach the server. A server that inherits the process
environment inherits every credential in it, and it never had to ask.

**`pin.version`** — an exact version. A floating tag means the server you reviewed and the
server you run are related only by name.

**`.mcp.json` is generated** from the allowlist by `agentarch sync --targets mcp_json`. Two
hand-kept files agree right up until one is edited, and the one that gets edited is never the
reviewed one.

## The prompt matters here too

Read the untrusted-content section of the system prompt. A tool description is untrusted input
arriving through a trusted channel — the same rule as a retrieved document, and the one people
most often forget because the text came from something they configured deliberately.
