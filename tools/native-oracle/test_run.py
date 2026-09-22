import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

import run


class OracleBoundaryTest(unittest.TestCase):
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
