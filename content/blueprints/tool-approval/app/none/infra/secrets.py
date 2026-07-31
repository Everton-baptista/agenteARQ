"""Where a secret's name becomes its value. The only place that happens.

Invariant 3 says a manifest references a secret by name and never carries the value. That leaves a
question the manifest cannot answer: who turns the name into the value. Here — one function, so
moving from environment variables to a secret manager is a change in one place rather than a search
for every `os.getenv` in the codebase.

Deliberately vendor-neutral. The blueprint depends on no cloud SDK, because a starting point that
requires an AWS account excludes everyone not on AWS, and a governance standard that ships an
opinion about which vault you use is overreaching. What it ships instead is the seam, with the three
common implementations written out as comments you can uncomment.

What must not change when you swap the backend:
  - the value is fetched once at startup, never per request
  - the value is never logged, never put in a span attribute, never returned by an endpoint
  - a missing secret is fatal at startup, not a 500 at request time
"""

from __future__ import annotations

import functools
import os


class SecretNotFound(RuntimeError):
    """A referenced secret does not resolve. Always fatal, and always at startup.

    Failing at startup rather than on first use is the difference between a deploy that does not
    go live and a deploy that goes live and fails every request while looking healthy.
    """


@functools.lru_cache(maxsize=32)
def resolve(name: str) -> str:
    """Return the value of the named secret, or raise.

    Cached for the process lifetime. A secret manager call per request is a latency floor and a
    bill, and rotation is handled by restarting or by a TTL you add here — not by fetching every
    time on the chance that it changed.
    """
    value = _from_environment(name)
    if value:
        return value

    raise SecretNotFound(
        f"{name} is not set.\n"
        "  local:      copy .env.example to .env and fill it in\n"
        "  container:  pass it as an environment variable from your orchestrator's secret store\n"
        "  production: implement one of the backends in infra/secrets.py\n"
        "Never commit the value — control.ai.api.secrets_not_committed blocks that, and a "
        "credential in git history outlives the commit that removed it."
    )


def _from_environment(name: str) -> str | None:
    val = os.getenv(name)
    if not val and name == "AWS_BEARER_TOKEN_BEDROCK_API_KEY":
        val = os.getenv("AWS_BEARER_TOKEN_BEDROCK")
    return val or None


# ── the three backends, written out rather than abstracted ───────────────────────────────────
#
# Each is ten lines. Uncomment one, add its SDK to requirements.txt, and call it from resolve()
# before falling back to the environment. Written as comments rather than as an installed plugin
# system because a plugin system for three ten-line functions is the kind of abstraction that
# costs more to understand than the thing it abstracts.
#
# AWS Secrets Manager:
#
#   import boto3
#   def _from_aws(name: str) -> str | None:
#       client = boto3.client("secretsmanager")
#       prefix = os.environ["SECRET_PREFIX"]          # e.g. "prod/rag-support/"
#       try:
#           return client.get_secret_value(SecretId=prefix + name)["SecretString"]
#       except client.exceptions.ResourceNotFoundException:
#           return None
#
# GCP Secret Manager:
#
#   from google.cloud import secretmanager
#   def _from_gcp(name: str) -> str | None:
#       client = secretmanager.SecretManagerServiceClient()
#       project = os.environ["GCP_PROJECT"]
#       path = f"projects/{project}/secrets/{name}/versions/latest"
#       try:
#           return client.access_secret_version(name=path).payload.data.decode()
#       except Exception:
#           return None
#
# HashiCorp Vault:
#
#   import hvac
#   def _from_vault(name: str) -> str | None:
#       client = hvac.Client(url=os.environ["VAULT_ADDR"], token=os.environ["VAULT_TOKEN"])
#       mount, path = os.environ["VAULT_MOUNT"], os.environ["VAULT_PATH"]
#       data = client.secrets.kv.v2.read_secret_version(mount_point=mount, path=path)
#       return data["data"]["data"].get(name)
#
# One note that applies to all three and is easy to get wrong: the IAM policy or Vault role must
# grant read on exactly the secrets this service needs, not on the path prefix. A wildcard grant
# turns one compromised service into every credential in the account, and it is the least-privilege
# rule from standard 03 applied to the thing that holds the keys.


def redact(value: str | None) -> str:
    """For the rare case where a secret has to appear in human-facing output at all.

    Shows enough to tell two credentials apart and not enough to use either. If you find yourself
    reaching for this in a log line, the log line is the problem.
    """
    if not value:
        return "(unset)"
    return f"{value[:3]}…{value[-2:]} ({len(value)} chars)"
