import io
from contextlib import redirect_stdout
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

import run


class OracleBoundaryTest(unittest.TestCase):
    def test_comparison_rejects_mutually_consistent_invalid_identity(self):
        # Re-hash the entire set after each mutation: checksums alone must not
        # make two stale or misbound receipts acceptable evidence.
        for mutation, reason in [('epoch', 'build epoch'), ('abi', 'C ABI'),
                                 ('abi_range', 'C ABI'), ('abi_bounds', 'C ABI'),
                                 ('header', 'C ABI'), ('bytes', 'input byte')]:
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory(dir=run.ROOT / '.scratch') as work:
                directory = Path(work) / 'records'
                directory.mkdir()
                cases = [c for c in run.read_json(run.ROOT / 'testdata/oracle/cases.json') if c['id'] == 'SM-GO']
                cases_path = Path(work) / 'cases.json'
                run.write_new(cases_path, cases)
                build = run.read_json(run.ROOT / 'testdata/oracle/windows-c/build.json')
                receipt = run.read_json(run.ROOT / 'testdata/oracle/windows-c/SM-GO.json')
                if mutation == 'epoch':
                    build['pins']['oracle']['runtime_commit'] = '0' * 40
                elif mutation == 'abi':
                    receipt['abi'] -= 1
                elif mutation == 'abi_range':
                    build['grammars']['go']['abi'] = receipt['abi'] = 1000
                elif mutation == 'abi_bounds':
                    for field in ('abi', 'runtime_abi_min', 'runtime_abi_max'):
                        build['grammars']['go'][field] = 999
                    receipt['abi'] = 999
                elif mutation == 'header':
                    build['grammars']['go']['runtime_header_sha256'] = '0' * 64
                else:
                    receipt['input_bytes'] += 1
                    receipt['end'] += 1
                run.write_new(directory / 'build.json', build)
                receipt['build_sha256'] = run.sha((directory / 'build.json').read_bytes())
                run.write_new(directory / 'SM-GO.json', receipt)
                run.write_new(directory / 'set.json', {'schema': 1, 'cases_sha256': run.sha(cases_path.read_bytes()),
                              'files': {p.name: run.sha(p.read_bytes()) for p in directory.iterdir()}})
                with redirect_stdout(io.StringIO()), self.assertRaisesRegex(ValueError, reason):
                    run.compare(directory, directory, cases_path)

    def test_comparison_detects_tampering_and_semantic_change(self):
        with tempfile.TemporaryDirectory(dir=run.ROOT / '.scratch') as work:
            directories = [Path(work) / side for side in ('left', 'right')]
            cases_path = Path(work) / 'cases.json'
            run.write_new(cases_path, [c for c in run.read_json(run.ROOT / 'testdata/oracle/cases.json') if c['id'] == 'SM-GO'])
            for directory in directories:
                directory.mkdir()
                for name in ('build.json', 'SM-GO.json'):
                    (directory / name).write_bytes((run.ROOT / 'testdata/oracle/windows-c' / name).read_bytes())
                run.write_new(directory / 'set.json', {'schema': 1, 'cases_sha256': run.sha(cases_path.read_bytes()),
                              'files': {p.name: run.sha(p.read_bytes()) for p in directory.iterdir()}})
            with redirect_stdout(io.StringIO()):
                self.assertTrue(run.compare(*directories, cases_path))
            record = directories[1] / 'SM-GO.json'
            data = run.read_json(record)
            data['has_error'] = True
            record.write_text(json.dumps(data), encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'record file changed'):
                run.compare(*directories, cases_path)
            index_path = directories[1] / 'set.json'
            index = run.read_json(index_path)
            index['files']['SM-GO.json'] = run.sha(record.read_bytes())
            index_path.write_text(json.dumps(index), encoding='utf-8')
            with redirect_stdout(io.StringIO()):
                self.assertFalse(run.compare(*directories, cases_path))
            cases_path.write_text('[]', encoding='utf-8')
            with self.assertRaisesRegex(ValueError, 'fixture identity mismatch'):
                run.compare(*directories, cases_path)

    def test_archive_and_fixture_boundaries(self):
        with tempfile.TemporaryDirectory(dir=run.ROOT / '.scratch') as work:
            folder = Path(work)
            for name, kind in [('repo/../escape.c', tarfile.REGTYPE),
                               ('repo/src/link.c', tarfile.SYMTYPE),
                               ('repo/C:/escape.c', tarfile.REGTYPE)]:
                buffer = io.BytesIO()
                with tarfile.open(fileobj=buffer, mode='w:gz') as archive:
                    member = tarfile.TarInfo(name)
                    member.type = kind
                    archive.addfile(member, io.BytesIO())
                with self.assertRaises(ValueError):
                    run.unpack(buffer.getvalue(), folder / 'extract', 'go')
                self.assertFalse((folder / 'extract').exists())
            buffer = io.BytesIO()
            with tarfile.open(fileobj=buffer, mode='w:gz') as archive:
                member = tarfile.TarInfo('repo/src/parser.c')
                member.size = 2
                archive.addfile(member, io.BytesIO(b'OK'))
            hashes = run.unpack(buffer.getvalue(), folder / 'extract', 'go')
            self.assertEqual(hashes, {'src/parser.c': run.sha(b'OK')})
            with self.assertRaises(ValueError):
                run.unpack(buffer.getvalue(), folder / 'extract', 'go')
            case = {'id': 'UTF8', 'source': '한글\r\n', 'sha256': run.sha('한글\r\n'.encode())}
            file = folder / 'cases.json'
            file.write_text(json.dumps([case]), encoding='utf-8')
            self.assertEqual(list(run.fixtures(file))[0][1], '한글\r\n'.encode())
            case['sha256'] = '0' * 64
            file.write_text(json.dumps([case]), encoding='utf-8')
            with self.assertRaises(ValueError):
                list(run.fixtures(file))
            run.write_new(folder / 'receipt.json', {'result': 'retained'})
            with self.assertRaises(FileExistsError):
                run.write_new(folder / 'receipt.json', {'result': 'overwrite'})


if __name__ == '__main__':
    unittest.main()
