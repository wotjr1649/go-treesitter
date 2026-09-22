# C oracle records

The Windows product test reads `testdata/oracle/windows-c-v2/` with CGO disabled.
It requires no compiler, Python, generator, container or network. Records carry
the source identity, build identity, complete ordered nodes, and their digest.
Active differences have exact Go and C hashes in `active-differences.json`;
the original `known-differences.json` preserves the pre-patch failures.
Changing either side fails the ratchet; an unregistered difference fails too.

Only the six documented KR-0001a bare-ampersand shapes remain active differences.
The C# order and preservation catalogs require exact nodes and error receipts.
`csharp-recovery-differences.json` retains the former KR-0004 signature as
historical evidence and is no longer an active exception. C roots may omit
leading hidden text; their reported span must match their first canonical node
and end at the full input length.

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

The approved TypeScript/TSX inputs under `testdata/oracle/typescript-patched/`
contain the entire maintained upstream grammar patch, its generated grammar
JSON, and scanner sources. Their manifest is bound by `identities.json`.
Building these two grammars requires the currently selected CLI and always
generates build-local C sources from that JSON. The original unpatched C sets
remain historical evidence; they are not the active TypeScript oracle.

To use an updated installed generator, pass `build --generate`. Selection uses
`TREE_SITTER_CLI` or PATH on each invocation, without a release constant. The
input is grammar JSON, so no grammar JavaScript or npm installation is needed.
Generation happens in a build-local copy; prepared source inputs stay intact.
The actual producer version, executable hash, generated sources, ABI, compiler
and binary are recorded. Unsupported ABI or CLI behavior fails the build.
A successful generation does not automatically adopt a new digest set.

Compare an existing and a newly produced set without adopting either:

```powershell
python tools/native-oracle/run.py compare --left testdata/oracle/windows-c-v2/base --right .scratch/oracle/records-new
```

The command checks both inventories and identities, reports the first ordered
difference, and fails on a changed tree or error state. Different producers and
ABIs keep their separate build identities even when all snapshots are equal.
Both builds must match the current pinned epoch. The independent
`testdata/oracle/runtime-abi.json` receipt binds the supported ABI range and
header hash to the pinned runtime commit, so mutually altered build metadata
cannot redefine that range. Fixture byte lengths are checked against actual
catalog inputs. This anchor changes only with an explicitly authorized runtime
epoch migration; it places no version restriction on the selected generator.

Add a fixture by registering its exact UTF-8 source or licensed testdata path,
hash, grammar, and filename in `cases.json`, then produce and review new records.
The C driver limits inputs to 4 MiB, parsing to 10 seconds, tree depth to 4096
and nodes to one million; the parent enforces an additional process timeout.

The driver also accepts four decimal arguments: old-source byte length, edit
start byte, old end byte, and new end byte. Supply old bytes followed by final
bytes on stdin (4 MiB combined). It validates ranges, unchanged prefix/suffix,
and UTF-8 edit boundaries before editing the old C tree. Output uses the same
ordered representation as a fresh parse. `check_edits.py` exercises this path
against fresh results and rejects malformed edit requests in the oracle lane:

```powershell
python tools/native-oracle/check_edits.py --executable .scratch/oracle/build-new/typescript.exe --cases path/to/edit-cases.json --filename x.ts
```

Each edit case supplies `ID`, `Filename`, `Previous`, `Source`, and `Edit` with
`StartByte`, `OldEndByte`, and `NewEndByte`. Build with the current driver before
running the check; an older executable does not implement this protocol.

`python -m unittest discover -s tools/native-oracle -v` checks tooling boundaries.
Session 06 initially found no installed CLI. Its later Phase 4 used a task-local
official portable CLI to exercise actual JSON regeneration, six C builds and
the 50-input comparison. The actual version is in that phase's receipt, never
a version constant in the runner. No synthetic parser output substitutes for it.
Linux and remote CI execution have independent receipts when run.
