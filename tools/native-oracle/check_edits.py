"""Exercise the real C driver in the separate oracle lane, including bad edits."""
import argparse
import json
from pathlib import Path
import subprocess


def check(executable, cases):
    count = 0
    for case in cases:
        before, after = case['Previous'].encode(), case['Source'].encode()
        edit = case['Edit']
        fresh = subprocess.run([str(executable)], input=after, capture_output=True,
                               check=True, timeout=15)
        command = [str(executable), str(len(before)), str(edit['StartByte']),
                   str(edit['OldEndByte']), str(edit['NewEndByte'])]
        incremental = subprocess.run(command, input=before + after, capture_output=True,
                                     check=True, timeout=25)
        if json.loads(fresh.stdout) != json.loads(incremental.stdout):
            raise AssertionError('C incremental/fresh difference: ' + case['ID'])
        count += 1
    for args, source in [(['-1', '0', '0', '0'], b''),
                         (['0', '1', '0', '0'], b'x'),
                         (['8', '0', '0', '0'], b'x'),
                         (['1', '0', '0', '0'], b'ab'),
                         (['2', '1', '1', '1'], 'éé'.encode()),
                         (['999999999999999999999', '0', '0', '0'], b'')]:
        result = subprocess.run([str(executable), *args], input=source,
                                capture_output=True, timeout=15)
        if result.returncode != 9 or result.stdout:
            raise AssertionError('invalid edit admitted: ' + repr(args))
    print(json.dumps({'incremental_fresh_equal': count, 'invalid_edits_rejected': 6}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--executable', required=True, type=Path)
    parser.add_argument('--cases', required=True, type=Path)
    parser.add_argument('--filename', required=True)
    options = parser.parse_args()
    cases = json.loads(options.cases.read_text(encoding='utf-8'))
    selected = [c for c in cases if c['Filename'] == options.filename and c.get('Previous') is not None]
    if not selected:
        parser.error('no matching edit cases')
    check(options.executable.resolve(), selected)
