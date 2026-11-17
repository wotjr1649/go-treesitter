import json
import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "native-ref"))
import proctree

RECORD = Path(__file__).with_name('record.py')


class RecordTest(unittest.TestCase):
    def test_timeout_keeps_partial_output_in_receipt(self):
        spec = importlib.util.spec_from_file_location('record', RECORD)
        record = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(record)
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'timeout.json'
            argv = ['record.py', '--output', str(output), '--', 'child']
            with patch.object(sys, 'argv', argv), patch.object(record, 'capture', return_value='a' * 40), \
                    patch.object(record, 'cpu_load', return_value=None), \
                    patch.object(record.proctree, 'run', side_effect=subprocess.TimeoutExpired('child', 7200, b'partial')):
                self.assertEqual(record.main(), 124)
            receipt = json.loads(output.read_text(encoding='utf-8'))
            self.assertEqual(receipt['exit_code'], 124)
            self.assertEqual(receipt['output_tail'], 'partial')
            self.assertEqual(receipt['timeout_seconds'], 7200)

    def test_receipt_keeps_failure_and_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'receipt.json'
            command = [sys.executable, str(RECORD), '--output', str(output), '--',
                       sys.executable, '-c', 'import sys; print("boom"); sys.exit(3)']
            self.assertEqual(proctree.run(command, capture_output=True, timeout=120).returncode, 3)
            receipt = json.loads(output.read_text(encoding='utf-8'))
            self.assertEqual(receipt['exit_code'], 3)
            self.assertIn('boom', receipt['output_tail'])
            self.assertEqual(len(receipt['commit']), 40)
            self.assertNotEqual(proctree.run(command, capture_output=True, timeout=120).returncode, 3)  # output exists


if __name__ == '__main__':
    unittest.main()
