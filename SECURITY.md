# Security policy

## Supported versions

This policy takes effect at 1.0. Until then the project is a Preview: only the latest released version gets fixes.

From 1.0, security and defect fixes go to:

- **Root module:** the latest 1.x minor and the preceding 1.x minor, if any (a 0.x Preview
  version is never supported this way).
- **Grammar modules:** the latest tag of each grammar module only. Grammar modules are versioned independently of the
  root ([ADR-0028](docs/design/decisions/ADR-0028-root-stability-and-independent-grammar-versioning.md)).
- **Go:** the two Go releases the Go team supports; the `go.mod` floor follows ADR-0028 decision 5.

Response time is best-effort. Released tags are never moved: a fix ships as a new tag, and a bad version is retracted
in its module's `go.mod` ([publication](docs/specs/publication.md)).

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

- Wrong or incomplete syntax trees from a pinned grammar. Report them as ordinary issues or to the grammar's
  repository; known ones are listed in [docs/known-gaps](docs/known-gaps).
- The documented limits in [limitations](docs/reports/limitations.md), for example an external scanner that never
  returns and therefore cannot be cancelled.
