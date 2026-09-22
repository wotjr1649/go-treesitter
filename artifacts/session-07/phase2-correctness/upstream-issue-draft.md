# C# recovery version order and reconstruction — unsent report

Baseline: gotreesitter v0.53.0, commit c871b1f576866c40b1695677fe3512243e266d39.
Native reference: tree-sitter v0.25.1, commit
f5afe475deb7c0bae6407fb776c76824f717bb61; C# grammar commit
88366631d598ce6595ec655ce1591b315cffb14c. Windows/amd64, CGO-free Go caller;
separately compiled native C driver. No source-epoch change.

The 52-byte enum witness in csharp-reduction-matrix-01.json (`members-2-0`)
contains two ordinary members, a conditional member with a trailing comma, and
the real #endif. Trace 03 shows a missing-token sibling already advanced to
position 39 in Go, while native C still has that sibling at 38. Go lets it
defeat the absorber's recovery at cost 702 versus 610; C's position test excludes
that sibling and reaches state 6874. The resulting preproc_if spans differ.

C# compatibility reconstruction is a second concern. For the licensed
JsonTextReader excerpt with only its enum section removed, Go reports
accepted_clean and 1,910 nodes; native C reports errors and 2,051 nodes. Source
SHA-256: d76ae3bbb63cc5c578a75fa14318dffe0dae30400de4e0a3ef9c19dfd5405ef4.
The source and complete C record are retained in the independent KR-0004 set.
Disabling all compatibility normalization reveals errors but also breaks a
directive-free control, so it is not a proposed fix.

The trace, three distinguishing controls and reduction matrix accompany this
draft. An experimental competition deferral confirms the enum mechanism but
leaves multiple full-excerpt differences. Please review recovery version
scheduling and source reconstruction independently. No production patch or
change to error-cost constants is proposed by this report.
