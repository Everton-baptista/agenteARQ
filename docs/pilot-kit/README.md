# Pilot kit

Materials for running the [lay-user pilot](../pilot-protocol.md). Participant-facing texts are
in the participant's language; Portuguese is provided, and other languages are translated from
it before the session, never improvised in it.

| File | Use |
|---|---|
| [roteiro-facilitador.pt-BR.md](roteiro-facilitador.pt-BR.md) | The facilitator's script: setup, the sentence read aloud, when to intervene, what to log |
| [termo-de-consentimento.pt-BR.md](termo-de-consentimento.pt-BR.md) | Consent form signed before recording starts |
| [log-template.csv](log-template.csv) | Copied to `pilot/<id>/log.csv` and filled during the session |

The facilitator log has one row per event. `kind` is one of:

| kind | When |
|---|---|
| `stop` | The participant says they would stop here at home. `step` names where (for example `doctor`, `verify`, `review`). |
| `intervention` | The facilitator helped. Any row of this kind makes the participant's result "aided". |
| `unclear_refusal` | A refusal appeared and its explanation was not enough to act on. |
| `bypass` | A gate was bypassed or a generated file or evidence was edited by hand. |
| `fabricated_reviewer` | A review was recorded under a name that did not review. |
| `note` | Anything else worth keeping. |

At the end of every session:

```bash
python3 scripts/pilot.py collect --product <participant's project folder> --participant P01 --session 1 --out pilot
```

After the last session of the cohort:

```bash
python3 scripts/pilot.py summarize --out pilot
```

It prints one row per participant and whether the cohort meets the protocol's measured criteria
(exit 0 when it does). Keep the `pilot/` folder outside any product repository; it holds names
only as participant ids.
