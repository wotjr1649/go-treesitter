import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

RECORD = Path(__file__).with_name('record.py')


class RecordTest(unittest.TestCase):
    def test_receipt_keeps_failure_and_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'receipt.json'
            command = [sys.executable, str(RECORD), '--output', str(output), '--',
                       sys.executable, '-c', 'import sys; print("boom"); sys.exit(3)']
            self.assertEqual(subprocess.run(command, capture_output=True).returncode, 3)
            receipt = json.loads(output.read_text(encoding='utf-8'))
            self.assertEqual(receipt['exit_code'], 3)
            self.assertIn('boom', receipt['output_tail'])
            self.assertEqual(len(receipt['commit']), 40)
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 3)  # output exists


if __name__ == '__main__':
    unittest.main()
