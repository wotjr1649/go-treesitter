# go-treesitter

A Go syntax-analysis layer over a pinned `gotreesitter` runtime, with explicit
parse outcomes, an adapter boundary, and Windows-native CGO-free validation.

This repository owns syntax results and their evidence. Language semantics,
code graphs, and downstream application policy belong to consumers.

Start with [AGENTS.md](AGENTS.md) and the [documentation map](docs/README.md).
The [validation contract](docs/specs/validation.md) separates development use
from release approval. No release is approved yet.
