"""The shared budget, and why it is shared.

Three agents with ten steps each is a system with thirty, and a delegation cycle between them is a
system with no bound at all. Per-agent limits do not catch A → B → A: each agent is inside its own
limit at every moment, and the system never terminates.

So there is one budget for the whole request, it is passed down through every handoff, and it counts
four things that per-agent limits cannot see: total steps, delegation depth, the path taken, and
total spend.

Its own module rather than a class inside the runner, because it is the thing every part of the
system has to agree about — and because the tests for termination are the tests that matter most here
and they should not need a runner to exercise.
"""

from __future__ import annotations

from dataclasses import dataclass, field


class BudgetExhausted(Exception):
    """The system has run out of a bound it declared. Always terminal, never retried.

    An exception rather than a return value: a caller that forgets to check a boolean produces the
    unbounded loop this module exists to prevent.
    """


@dataclass
class Budget:
    max_steps: int
    max_depth: int = 3
    max_usd: float = 1.0

    used: int = 0
    depth: int = 0
    spent_usd: float = 0.0
    # The delegation path, which is what makes a cycle visible. A set would answer "have we been
    # here", but the list also answers "how did we get here", and that is what an operator reads
    # when a request went somewhere unexpected.
    path: list[str] = field(default_factory=list)

    def spend_step(self, agent_id: str) -> tuple[bool, str]:
        """Account for one step about to be taken by `agent_id`."""
        self.used += 1
        if self.used > self.max_steps:
            return False, f"shared step budget exhausted after {self.used} steps"
        if self.depth > self.max_depth:
            return False, f"delegation deeper than {self.max_depth}"
        return True, ""

    def may_delegate_to(self, agent_id: str) -> tuple[bool, str]:
        """Whether handing off to `agent_id` is allowed right now.

        The cycle check is here rather than in spend_step because an agent appearing twice in one
        path is only a cycle when the second appearance is a delegation. An orchestrator that is
        re-entered after its specialist returns is a normal return, not a loop.
        """
        if agent_id in self.path:
            return False, f"delegation cycle: {' → '.join(self.path)} → {agent_id}"
        if self.depth + 1 > self.max_depth:
            return False, f"delegation deeper than {self.max_depth}"
        return True, ""

    def spend_usd(self, amount: float) -> tuple[bool, str]:
        self.spent_usd += amount
        if self.spent_usd > self.max_usd:
            return False, f"shared cost budget of ${self.max_usd} exhausted"
        return True, ""

    def enter(self, agent_id: str) -> None:
        self.depth += 1
        self.path.append(agent_id)

    def leave(self) -> None:
        self.depth -= 1
        if self.path:
            self.path.pop()

    def snapshot(self) -> dict:
        """For the response and for a span attribute.

        Returned to the caller because a multi-agent system that reports only its answer gives an
        operator no way to see that one request quietly used nine steps across three agents.
        """
        return {
            "steps_used": self.used,
            "max_steps": self.max_steps,
            "depth": self.depth,
            "path": list(self.path),
            "spent_usd": round(self.spent_usd, 6),
            "max_usd": self.max_usd,
        }
