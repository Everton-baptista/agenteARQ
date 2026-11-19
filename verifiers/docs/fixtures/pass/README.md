# Fixture documentation

Start with [the guide](docs/guide.md) or browse [the docs folder](docs/).
See [installation](docs/guide.md#install) and the [project site](https://example.com/docs).
A reference link to [the diagram][diagram] and an image:

![Architecture](docs/img/architecture.svg)

<img src="docs/img/architecture.svg" alt="Architecture again" width="200">

```mermaid
flowchart LR
  A[Link inside code](missing-in-code.md) --> B
```

Inline code is not a link: `[x](not-a-file.md)`.

[diagram]: docs/img/architecture.svg "Architecture diagram"
