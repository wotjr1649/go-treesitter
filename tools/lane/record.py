"""Run one local check command and write a receipt of its conditions and result.

Usage: python tools/lane/record.py --output NEW.json -- go test -p 1 ./... -count=1
The receipt binds the commit, tree state, Go version, command, host CPU load
(before and after, via pwsh) and the exit code with the full output's hash.
It never retries and refuses to overwrite an existing receipt.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]


def capture(command):
    try:
        return subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=60).stdout.strip()
    except (OSError, subprocess.TimeoutExpired):
        return None


def cpu_load():
    value = capture(['pwsh', '-NoProfile', '-Command',
                     '(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average'])
    return float(value) if value else None


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('command', nargs=argparse.REMAINDER)
    options = parser.parse_args()
    command = options.command[1:] if options.command[:1] == ['--'] else options.command
    if not command:
        parser.error('a command is required after --')
    with options.output.open('x', encoding='utf-8', newline='\n') as stream:
        receipt = {'commit': capture(['git', 'rev-parse', 'HEAD']),
                   'tree_clean': capture(['git', 'status', '--porcelain']) == '',
                   'go_version': capture(['go', 'version']), 'command': command,
                   'cpu_load_percent_before': cpu_load()}
        started = time.time()
        result = subprocess.run(command, cwd=ROOT, capture_output=True)
        output = result.stdout + result.stderr
        receipt.update(seconds=round(time.time() - started, 3), exit_code=result.returncode,
                       cpu_load_percent_after=cpu_load(),
                       output_sha256=hashlib.sha256(output).hexdigest(),
                       output_tail=output[-16384:].decode('utf-8', 'replace'))
        json.dump(receipt, stream, indent=2)
        stream.write('\n')
    print(json.dumps({k: receipt[k] for k in ('exit_code', 'seconds', 'cpu_load_percent_before', 'cpu_load_percent_after')}))
    return result.returncode


if __name__ == '__main__':
    raise SystemExit(main())
