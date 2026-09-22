# Local patch and fork recommendation

Retain the minimal JSX-text `=` candidate for an upstream submission. Its
isolated identity, fresh 94-input comparison and four edit checks are complete
in this phase. Keep the `/` heuristic and bare `&` behavior unchanged. Retire
the candidate only after an authorized tagged baseline contains the equivalent
correction and the affected differential/incremental tests pass again.

A remote fork is not currently necessary to preserve this work. Prefer an
upstream contribution with the small scanner diff and exact witnesses. No
issue/comment/PR has been sent. The open report does not tell us whether the
maintainer intends to fix it. A tagged fix still needs the existing deliberate
baseline-upgrade procedure; the installed generator can continue evolving
independently under its artifact identity checks.

Do not adopt the C# diagnostic patches. They alter scheduling and source-based
reconstruction, still fail full ordered comparisons, and have not passed the
upstream multi-language recovery campaigns. The proper target is version order
and genuine recovery preservation inside the runtime, not adapter AST rewriting.
The local reproducer and trace identify the first reduced divergence for an
upstream investigation without claiming a complete scheduler repair.

KR-0003 needs a separate oracle-definition decision. Upstream's TypeScript
grammar patch adds behavior absent from the original C sources. Applying that
patch only to make these tests pass would change artifact semantics under the
existing epoch. Retain both original-source observations and the distinction;
do not use patched and unpatched C outputs interchangeably.

If a future release deadline requires a maintained runtime fork, first settle
ownership, permitted patch identities, upstream synchronization and retirement.
A Go library's local replace directive is ignored by consuming main modules;
it does not deliver a patched dependency to users. A real distribution would
need a deliberately owned module/import path or an explicit consumer build
arrangement, plus the same provenance, fresh/incremental C, resource and native
platform checks. That is a product dependency decision, beyond the current
isolated-experiment authorization. No fork, vendor substitution or product
replacement was performed here.
