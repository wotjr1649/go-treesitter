# Candidate packaging check

Run from a clean committed Windows checkout with Go 1.27.1, Python and Git:

```powershell
New-Item -ItemType Directory -Force .scratch | Out-Null
go -C tools/modulezip build -mod=readonly -o ../../.scratch/modulezip.exe .
python -m unittest discover -s tools/release -p 'test_*.py'
python tools/release/check.py .scratch/modulezip.exe
python tools/licenses/check.py
```

The nested `tools/modulezip` module pins the `golang.org/x/mod` revision used
by Go 1.27.1. Only building this tool may fetch that public dependency. It is
excluded from both product module ZIPs and from consumer dependency graphs.

The packaging check uses `zip.CreateFromVCS` and `zip.CheckZip`, validates
notices/provenance and the 203+3 grammar split, then builds and executes the
retained consumers through a local file proxy with fresh module caches and no
replace directives. It rejects CGO dependencies and executes all 206 basic
catalog cases against those same packaged modules. Scratch outputs and failure
logs remain local; `artifacts/release-ci/packaging.json` contains the reviewed
public receipt. It refuses to overwrite that receipt. A fresh checkout supplies
an empty output directory for each CI run.

A packaging PASS does not bypass the independent license check, native platform
gates or the seven-route C differential. No command publishes a module or tag.
