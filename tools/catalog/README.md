# Basic catalog checks

Build the public API probe with `CGO_ENABLED=0`, then run each of the 206 recorded
language samples in a separate process (two concurrent processes maximum):

```powershell
$env:CGO_ENABLED = '0'
go build -o .scratch/catalog.exe ./tools/catalog
python tools/catalog/check.py --executable .scratch/catalog.exe --output .scratch/catalog-results.json
```

The output file must not exist. Each case checks three clean deterministic fresh
parses, a newline edit against a fresh parse, pre-cancellation, the input byte
limit and idempotent tree release. Each parse has a two-second timeout, a 1 MiB
input limit, a 100,000-node snapshot limit and a 64 MiB runtime allocation budget.
The process timeout is 30 seconds. The runtime budget is not a process RSS cap.

`testdata/catalog/basic.json` preserves `ParseSmokeSamples` from the pinned
gotreesitter v0.53.0 runtime and their SHA-256 hashes. Its MIT notice is retained
in `internal/runtime/LICENSE`. These samples establish basic catalog coverage;
they do not establish C oracle parity. The seven Release-Critical paths retain
their separate strict oracle gates.
