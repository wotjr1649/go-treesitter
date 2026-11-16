# Security policy

## Supported versions

This project is a preview. Only the latest released version gets fixes; earlier tags are not patched (released tags
are never moved, see [publication](docs/specs/publication.md)).

## Reporting a vulnerability

Report privately through GitHub: **Security → Report a vulnerability** on this repository. Do not open a public issue
for a suspected vulnerability. Include the module and version (or commit), the grammar, a minimal input and what you
observed. Reports are handled on a best-effort basis; there is no guaranteed response time.

## In scope

- Memory safety or a Go panic escaping the API, in the root `treesitter` package, `internal/engine` or a generated
  grammar module.
- A resource limit (`WithMaxInputBytes`, `WithMaxSnapshotNodes`, memory growth) that can be bypassed.
- Generated code or release assets that do not match the recorded pins and provenance.

## Out of scope

- Wrong or incomplete syntax trees from an upstream grammar. Report them as ordinary issues or to the grammar's
  repository; known ones are listed in [docs/known-gaps](docs/known-gaps).
- The documented limits in [limitations](docs/reports/limitations.md), for example an external scanner that never
  returns and therefore cannot be cancelled.
