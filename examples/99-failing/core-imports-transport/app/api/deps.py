"""The transport layer. Importing agent, domain and infra from here is correct and expected — the
arrow only points one way."""


def current_principal(authorization: str) -> str:
    return authorization.split(" ")[-1] or "anonymous"
