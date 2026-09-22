"""Run every recorded basic catalog case in a bounded, separate process."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--executable', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--cases', type=Path, default=ROOT / 'testdata/catalog/basic.json')
    options = parser.parse_args()
    executable = options.executable.resolve(strict=True)
    inputs = options.cases.read_bytes()
    cases = json.loads(inputs)['cases']
    if len(cases) != 206 or len({case['language'] for case in cases}) != 206:
        parser.error('expected the complete 206-language catalog')
    for case in cases:
        if sha(case['source'].encode()) != case['sha256']:
            parser.error('input hash mismatch: ' + case['language'])

    def check(case):
        row = {'language': case['language'], 'source_sha256': case['sha256']}
        try:
            result = subprocess.run([str(executable), '-language', case['language'], '-repeat', '3'],
                                    input=case['source'].encode(), capture_output=True, timeout=30)
            row.update(exit=result.returncode, stderr=result.stderr.decode(errors='replace')[:8192])
            try:
                row['receipt'] = json.loads(result.stdout)
            except ValueError:
                row.update(exit='INVALID_RECEIPT', stdout=result.stdout.decode(errors='replace')[:8192])
        except subprocess.TimeoutExpired:
            row['exit'] = 'TIMEOUT'
        return row

    # Reserve the receipt before starting work; never replace previous evidence.
    with options.output.open('x', encoding='utf-8', newline='\n') as output:
        with ThreadPoolExecutor(max_workers=2) as pool:
            rows = list(pool.map(check, cases))
        json.dump({'executable_sha256': sha(executable.read_bytes()),
                   'cases_sha256': sha(inputs), 'repeats': 3, 'parallel_processes': 2,
                   'rows': rows}, output, indent=2)
        output.write('\n')
    failed = [row['language'] for row in rows if row['exit'] != 0]
    print(json.dumps({'cases': len(rows), 'passed': len(rows) - len(failed), 'failed': failed}))
    return bool(failed)


if __name__ == '__main__':
    raise SystemExit(main())
