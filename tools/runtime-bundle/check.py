"""Exercise private runtime invariants without adding files to the pinned carrier."""
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SCRATCH = ROOT / '.scratch'
SCRATCH.mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='runtime-invariants-', dir=SCRATCH) as directory:
    overlay = Path(directory) / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {
        str(ROOT / 'internal/runtime/recovery_hardening_test.go'):
        str(ROOT / 'tools/runtime-bundle/testdata/recovery_hardening_test.go')
    }}), encoding='utf-8')
    result = subprocess.run(['go', 'test', '-overlay=' + str(overlay), './internal/runtime',
                             '-run', '^Test(SharedClosedRecoveryPrefix|RecoveryAcceptanceOrder|EOFTransitions|RecoveryRawLeafCost)$',
                             '-count=1', '-timeout=30s', '-json'],
                            cwd=ROOT, capture_output=True, timeout=90)
    print(result.stdout.decode(), end='')
    if result.stderr:
        print(result.stderr.decode(), end='')
    result.check_returncode()
    passed = {row.get('Test') for row in map(json.loads, result.stdout.splitlines()) if row.get('Action') == 'pass'}
    if not {'TestSharedClosedRecoveryPrefix', 'TestRecoveryAcceptanceOrder', 'TestEOFTransitions', 'TestRecoveryRawLeafCost'} <= passed:
        raise RuntimeError('private runtime invariant tests did not run')
