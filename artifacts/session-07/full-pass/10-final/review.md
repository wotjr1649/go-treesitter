# Recovery hardening work-unit review

Product runtime manifest after the last change:
`5d1f526c35e223341914cbfada8abe1c4c49d52824a09950bde05d331ff41e46`.
Origin remains `gotreesitter v0.53.0`, commit
`c871b1f576866c40b1695677fe3512243e266d39`; C runtime remains v0.25.1.

The Go blob now derives from the already pinned C tables. EOF/NUL terminals,
hidden missing-token cost, version-specific recovery lookahead, JSX text,
incremental dependencies and conflicting leaf reuse have retained regression
coverage. Original failing C evidence and minimized fuzz seeds are preserved.
The active C difference map is empty; no tree is normalized to conceal a
difference. `syntax.Index` accepts the anonymous NUL terminal while preserving
root/type, parent and range rejection controls.

The private cache test also binds raw child reuse to its captured shape: a
live node changed to a clean shape must not replace the hidden missing cost
captured by an older parent. This prevents a performance shortcut from
reintroducing false-clean recovery. It uses the existing bounded memo.

Observed checks for this work unit:

- E3/E5: `raw-child-memo-correctness`, all retained adapter tests, including
  the 688 C records and 439 edited records; the candidate changes only cost
  computation and all ordered comparisons pass.
- E3: `raw-child-memo-private` and `raw-child-product-final` pass. The earlier
  `raw-child-product` failure was the external consumer test's exact stdout
  expectation after adding architecture output. Updating that exact expected
  receipt fixed it; `raw-child-consumer-receipt` and the full suite pass.
- E4: five paired 150-iteration measurements reduce TSX delete median from
  7.81 to 3.80 ms and C# recovery from 12.20 to 10.91 ms. These compare the
  immediately preceding candidate to the raw-child memo change. They are not
  the final baseline/release comparison.
- Reproduction: the maintained patch inventory reproduces all 1,508 carrier
  files. The derived Go artifact has separate origin/product hashes and its
  pinned MIT C source archive is retained.

The `10-final/oracle`, resource, race and original five-seed performance results
precede the last raw-child memo patch and are not transferred to it. A complete
post-commit final campaign follows in `11-release/`. The earlier 60-second edit
fuzz result also precedes this last patch. This work unit makes no E6 claim.

Self-review covered all changed production branches, shared callers, ownership,
raw/captured cache identity, cancellation checks, generated-source reproduction,
negative controls and public compatibility. No debug instrumentation, source
special-case rewrite, weakened assertion, new dependency or global cache was
introduced. See `../09-audit/review.md` for external and downstream review scope
and the explicit independent-review limitation.

`hardening-command-output.zip` preserves the investigation harnesses and raw
logs at its snapshot time, including the superseded unadopted lookahead patch.
The final release archive will also bind complete closing logs. Failed attempts
remain evidence; abandoned candidate source is not in the product inventory.
