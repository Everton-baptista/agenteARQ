"""Retrieval, and the boundary that keeps what it returns from becoming an instruction.

Everything this module produces is untrusted content. Anyone who can edit a help centre article
can write into it — which is exactly why it is rendered as delimited data and never joined to the
system prompt. That is invariant 2, and the reason it is an invariant rather than advice is that
the f-string which breaks it is always one line away and always looks harmless.
"""

from __future__ import annotations

CORPUS = [
    {"id": "kb-001", "text": "Orders ship within two business days. Tracking arrives by email."},
    {"id": "kb-002", "text": "Returns are accepted within 30 days of delivery, unopened."},
    {"id": "kb-003", "text": "Delivery estimates come from the carrier and can change."},
]


def retrieve(question: str, top_k: int = 3) -> list[dict]:
    """Replace this with your retriever.

    Keep the return shape: an id per passage is what makes a citation checkable. An answer that
    cites nothing verifiable is indistinguishable from one that made the citation up.
    """
    # Prefix matching rather than exact words, so "order" finds "Orders" and "shipping" finds
    # "ship". Crude, and deliberately so — but a placeholder that returns nothing for the question
    # in the README teaches the reader that the agent does not work.
    terms = [t.strip("?.,!") for t in question.lower().split() if len(t) > 3]

    def score(text: str) -> int:
        words = text.lower().replace(".", " ").split()
        return sum(1 for t in terms if any(w.startswith(t[:4]) for w in words))

    ranked = sorted(((score(p["text"]), p) for p in CORPUS), key=lambda x: -x[0])
    return [p for s, p in ranked if s > 0][:top_k]


def render_untrusted(passages: list[dict], question: str) -> str:
    """Instruction and data, structurally apart.

    The tags mean nothing unless the system prompt gives them meaning — which it does, in the
    section saying that instructions appearing inside them are evidence of tampering rather than
    something to follow.
    """
    body = "\n".join(f"[{p['id']}] {p['text']}" for p in passages) or "(no passages found)"
    return (
        "<retrieved_content>\n" + body + "\n</retrieved_content>\n"
        "<customer_message>\n" + question + "\n</customer_message>"
    )
