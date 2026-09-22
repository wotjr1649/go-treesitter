# Native TSX diagnostic maintenance

The optional oracle_probe test still expected the retired bare-equals failure.
It now requires F4/F5 to be clean and node-identical to the adopted C oracle,
while F1-F3 retain their bare-ampersand characterization. The C probe includes
stdlib.h for its existing free call. These changes tighten the diagnostic.

The probe was rebuilt with the pinned C runtime and Candidate C parser, whose
hashes are recorded in native-tsx-probe.json. The CGO-disabled Go diagnostic
passed all seven cases (native-tsx-probe-check.json). This test invokes a C
executable in the separate oracle lane; no C toolchain enters product builds.
Self-review covered the exact fixture ordering, expected error flags, full
canonical-node comparison for clean cases, and C header/build identity.
