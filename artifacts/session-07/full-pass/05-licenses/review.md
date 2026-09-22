# Catalog distribution and license inventory

The owner's selected policy is implemented as 203 base-module grammars plus
three grammars in the separate optional GPL module. The same 206 upstream
blob identities are retained. The seven Release-Critical routes are unchanged.
The base imports no optional module. GPL-specific blobs, scanners, external
lex-state tables and queries are absent from its carrier. The source separator
checks exact declarations and file counts rather than silently omitting matches.

An early optional-module probe exposed scanner adaptation reading its baseline
blob through the aggregate provider. `catalog-optional-blob-providers.patch`
lets an explicit named provider use the existing loader/cache before the
aggregate reader. Its regression witness is the optional Caddy/Disassembly
parse, including concurrent first use. No separate cache or parser backend is
introduced. The first Windows workspace attempt also exposed slash-sensitive
relative package resolution; native path spellings supplied through a local
workspace passed. External consumers use no workspace or replace directive.

Observed product checks: `license-full-tests.json` passes `go test ./...` with
CGO disabled; `license-vet.json` passes; `license-gpl-tests.json` passes optional
module parsing, eight concurrent workers, provenance and absence of a replace.
`license-consumer-tests.json` installs both v0.0.1 module variants from empty
local caches and rejects consumer imports of the internal adapter. The main
consumer graph contains two modules; the GPL consumer contains three.
`catalog-206.json` records 206/206 basic cases after the split (E3).

197 fixed grammars have original notice texts. Nine rely on fixed SPDX package
declarations. Full recursive Git trees for those nine contain no license,
copyright, copying or notice paths. The raw metadata, source URLs and hashes
are retained. Crosschecking Cargo metadata found two real inconsistencies:
Brightscript and Cooklang declare ISC in package.json and MIT in Cargo.toml.
The public-source reviewer also checked current tips; no later explicit
license grant was found. These are open release licensing questions, preserved
as LicenseRef-Upstream-MIT-ISC-Conflict. This is not an assertion that either
project has no license, nor an invented dual-license grant.

The independent public-source review covered fixed commits and later explicit
license candidates without receiving private repository data. Primary raw
metadata and recursive trees were independently downloaded and hash-bound in
`crosscheck/`. Main/MIT-centered distribution does not mean every component
is MIT: Nim's MPL-2.0 source and Wat's Apache/LLVM exception are included.

The GPL module includes complete grammar sources plus the exact upstream
converter source and dependency. Converter builds and all three native
regenerations ran. Their blobs differ from the older bundled artifacts in
Gob schema and four decoded metadata fields, documented in the optional
module's source README. Every other exported Language field compared equal.
The failed strict byte/field comparisons are preserved; regenerated policy
metadata was not adopted. Import reproduction from the pinned module archive
remains a separate, exact 1,508-file main / eight-file optional comparison.

Reviewed module boundaries, init ordering, generated output, failure behavior,
source licenses, native paths and all changed source. Source input to the
importer is hash-checked; archive paths, duplicate entries and output ownership
are bounded. The optional module keeps runtime types inside its own adapter.
No remote write, release, tag, origin upgrade or change under `_ref` occurred.
Full official module-ZIP inspection and remaining resource/platform campaigns
are subsequent integrated checks, not claims made by this work unit.
