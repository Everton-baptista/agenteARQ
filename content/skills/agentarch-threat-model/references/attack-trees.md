# Attack trees

Four paths that apply to nearly every agent. Walk each one against the specific agent, and say
whether what stops it is a control that exists or an intention someone stated.

## 1. Indirect injection through retrieved content

```
Attacker wants the agent to act against the user
└── get text into the context
    ├── edit a document the corpus indexes          ← usually the easiest
    ├── get a tool to return attacker-controlled text
    ├── plant it in memory for a later session
    └── send it through another agent's handoff
        └── model treats it as instruction
            ├── STOPPED BY: content isolation (structural — it blocks)
            ├── REDUCED BY: injection screening (probabilistic — it does not)
            └── CONTAINED BY: tool least privilege
                └── damage = what the narrowest reachable tool can do
```

The last line is the one that matters. Prevention is probabilistic; containment is not.

## 2. Exfiltration through a tool

```
Attacker wants data out
├── a tool that calls out
│   └── STOPPED BY: egress allowlist naming exact hosts
├── a query parameter on an ALLOWED host
│   └── egress allowlist passes; the data still leaves
├── a URL the client will fetch
│   ├── markdown image in the reply
│   └── a link the user clicks
│       └── STOPPED BY: output guardrail stripping external targets
├── a DNS lookup of an attacker-chosen name
│   └── carries the payload even when the request fails
└── an error message echoed with attacker content in it
```

An egress allowlist is necessary and not sufficient. Where secrets are in scope, constrain what
may appear in **output** as well.

## 3. Confused deputy

```
Attacker cannot reach a system directly
└── persuade the agent to reach it
    ├── identity taken from model output      ← the common one, and it reads as good API design
    │   └── STOPPED BY: identity from the session, server-side, always
    ├── a tool running with a service account rather than the user's rights
    └── credentials shared across tools or servers
        └── STOPPED BY: per-tool and per-server credential scoping
```

## 4. MCP tool poisoning and rug pull

```
Attacker controls or compromises a server the agent uses
├── instructions hidden in a tool description
│   └── the model treats them as authoritative; the user never sees them
│       └── STOPPED BY: reviewing descriptions, and tool least privilege so the
│                       instruction has nothing worth reaching
├── benign at review, hostile afterwards            ← the rug pull
│   └── STOPPED BY: tool_description_sha256 + `agentarch mcp audit --probe`
├── a description that changes how a DIFFERENT server's tool is used
│   └── STOPPED BY: per-server tools_allow, and treating any cross-server
│                   instruction in a description as a finding
└── new tools appearing in a minor release
    └── STOPPED BY: an exact version pin and an enumerated tools_allow
```

## Using these

For each leaf, write one line in the threat model: is the path open for this agent, what closes
it, and does that thing exist. A leaf you cannot answer is a finding, not an omission.
