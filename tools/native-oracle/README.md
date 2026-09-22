# C oracle records

The Windows product test reads `testdata/oracle/windows-c` with CGO disabled.
It requires no compiler, Python, generator, container or network. Records carry
the source identity, build identity, complete ordered nodes, and their digest.
Known differences have exact Go and C hashes in `known-differences.json`.
Changing either side fails the ratchet; an unregistered difference fails too.

Produce records from the repository root with an installed C compiler:

```powershell
python tools/native-oracle/run.py prepare
python tools/native-oracle/run.py build --output .scratch/oracle/build-new
python tools/native-oracle/run.py record --build .scratch/oracle/build-new --output .scratch/oracle/records-new
```

Only `prepare` downloads public pinned sources. It never installs a tool.
`CC` selects an installed compiler. Builds reject changed source sets/hashes;
recording rejects changed executables, epochs, fixture bytes and existing output
directories. Review new output before explicitly adopting it in testdata.

To use an updated installed generator, pass `build --generate`. Selection uses
`TREE_SITTER_CLI` or PATH on each invocation, without a release constant. The
input is grammar JSON, so no grammar JavaScript or npm installation is needed.
Generation happens in a build-local copy; prepared source inputs stay intact.
The actual producer version, executable hash, generated sources, ABI, compiler
and binary are recorded. Unsupported ABI or CLI behavior fails the build.
A successful generation does not automatically adopt a new digest set.

Add a fixture by registering its exact UTF-8 source or licensed testdata path,
hash, grammar, and filename in `cases.json`, then produce and review new records.
The C driver limits inputs to 4 MiB, parsing to 10 seconds, tree depth to 4096
and nodes to one million; the parent enforces an additional process timeout.

`python -m unittest discover -s tools/native-oracle -v` checks tooling boundaries.
The installed generator was unavailable in the Session 06 environment; actual
regeneration is NOT_RUN, and no synthetic parser output substitutes for it.
Linux and remote CI execution have independent receipts when run.
