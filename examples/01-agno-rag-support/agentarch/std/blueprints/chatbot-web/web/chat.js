/* The browser half of the governed chat.
 *
 * Two rules hold here and are tested in app/tests:
 *
 *   1. The only URLs this file ever fetches are our own /v1 routes. The browser never talks
 *      to the model provider — every guardrail, budget and audit trail is server-side, and a
 *      page that could reach the provider directly would bypass all of them.
 *   2. No credential ships in this file. The token is typed by the visitor, kept in
 *      sessionStorage (this tab only), and sent as a Bearer header to /v1. View-source shows
 *      nothing, because there is nothing to show.
 */
"use strict";

const TOKEN_KEY = "agentarch.chat.token";
const CONVERSATION_KEY = "agentarch.chat.conversation";

const els = {
  tokenForm: document.getElementById("token-form"),
  tokenInput: document.getElementById("token-input"),
  sessionLabel: document.getElementById("session-label"),
  askForm: document.getElementById("ask-form"),
  questionInput: document.getElementById("question-input"),
  sendButton: document.getElementById("send-button"),
  messages: document.getElementById("messages"),
  statusText: document.getElementById("status-text"),
  costText: document.getElementById("cost-text"),
  approvalTemplate: document.getElementById("approval-template"),
};

let token = sessionStorage.getItem(TOKEN_KEY) || "";
let conversationId =
  sessionStorage.getItem(CONVERSATION_KEY) || crypto.randomUUID();
sessionStorage.setItem(CONVERSATION_KEY, conversationId);

function setConnected(connected) {
  els.tokenForm.hidden = connected;
  els.askForm.hidden = !connected;
  els.sessionLabel.hidden = !connected;
  if (connected) {
    els.sessionLabel.textContent = "connected";
    els.questionInput.focus();
  } else {
    els.statusText.textContent = "";
    els.tokenInput.focus();
  }
}

function addMessage(role, text) {
  const li = document.createElement("li");
  li.className = `msg msg-${role}`;
  li.textContent = text;
  els.messages.appendChild(li);
  li.scrollIntoView({ block: "end" });
  return li;
}

function setBusy(busy) {
  els.sendButton.disabled = busy;
  els.questionInput.disabled = busy;
  els.statusText.textContent = busy ? "agent is working…" : "";
}

function showCost(usd) {
  els.costText.textContent = usd > 0 ? `this turn: $${usd.toFixed(4)}` : "";
}

/* The one request helper. Every call goes through here so there is exactly one place the
 * Bearer header is set and one place a 401 is handled — a token that stops working should
 * look the same everywhere. */
async function api(path, body) {
  const r = await fetch(path, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
  });
  if (r.status === 401) {
    sessionStorage.removeItem(TOKEN_KEY);
    token = "";
    setConnected(false);
    addMessage("system", "That token was refused. Reconnect with a valid one.");
    throw new Error("unauthorized");
  }
  if (r.status === 429) {
    addMessage(
      "system",
      "You reached the daily limit for this account. The budget is a control, not a bug — " +
        "it is what a runaway cannot spend past."
    );
    throw new Error("budget exhausted");
  }
  if (!r.ok) {
    addMessage("system", `The service answered ${r.status}. Try again in a moment.`);
    throw new Error(`http ${r.status}`);
  }
  return r.json();
}

function renderCitations(li, citations) {
  if (!citations || citations.length === 0) return;
  const small = document.createElement("small");
  small.className = "citations";
  small.textContent = "sources: " + citations.map((c) => c.passage_id).join(", ");
  li.appendChild(small);
}

function renderApproval(approvalId, preview) {
  const card = els.approvalTemplate.content.firstElementChild.cloneNode(true);
  const detail = card.querySelector(".approval-detail");
  const rows = [
    ["tool", preview.tool],
    ["effect", `${preview.effect} — cannot be undone`],
    ["acting for", preview.acting_for],
  ];
  for (const [k, v] of Object.entries(preview.arguments || {})) {
    rows.push([k, String(v)]);
  }
  for (const [k, v] of rows) {
    if (v === undefined || v === null || v === "") continue;
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = v;
    detail.append(dt, dd);
  }
  card.querySelector(".approval-timeout").textContent =
    `If nobody decides, the action is denied automatically (${preview.on_timeout || "on timeout: deny"}).`;

  card.addEventListener("click", async (event) => {
    const button = event.target.closest("button[data-decision]");
    if (!button) return;
    for (const b of card.querySelectorAll("button")) b.disabled = true;
    try {
      const result = await api(`/v1/approvals/${approvalId}`, {
        decision: button.dataset.decision,
        reason: button.dataset.decision === "deny" ? "denied from the chat UI" : "",
      });
      card.remove();
      renderOutcome(result);
    } catch {
      card.remove(); // the error path already posted a system message
    }
  });

  const li = addMessage("agent", "");
  li.appendChild(card);
}

function renderOutcome(result) {
  if (result.status === "awaiting_approval") {
    renderApproval(result.approval_id, result.approval || {});
  } else {
    const li = addMessage(result.status === "escalated" ? "system" : "agent", result.answer);
    renderCitations(li, result.citations);
  }
  showCost(result.cost_usd || 0);
}

els.tokenForm.addEventListener("submit", (event) => {
  event.preventDefault();
  token = els.tokenInput.value.trim();
  if (!token) return;
  sessionStorage.setItem(TOKEN_KEY, token);
  els.tokenInput.value = "";
  setConnected(true);
});

els.askForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const question = els.questionInput.value.trim();
  if (!question) return;
  els.questionInput.value = "";
  addMessage("user", question);
  setBusy(true);
  try {
    const result = await api("/v1/ask", {
      question,
      conversation_id: conversationId,
    });
    renderOutcome(result);
  } catch {
    /* the error path already posted a system message */
  } finally {
    setBusy(false);
    els.questionInput.focus();
  }
});

setConnected(token !== "");
