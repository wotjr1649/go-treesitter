import tempfile
import unittest
from pathlib import Path
from identity import generate, identity


class ArtifactIdentityTest(unittest.TestCase):
    def test_generator_cannot_write_outside_repository(self):
        # TEMP may itself be repository-local in a contained test runner.
        # No file or directory is created at this deliberately external path.
        outside = Path(__file__).resolve().parents[3] / 'oracle-boundary-test'
        with self.assertRaisesRegex(ValueError, 'inside this repository'):
            generate(outside / 'api.h', outside / 'src/parser.c')

    def test_runtime_abi_boundaries_and_unpinned_producer(self):
        with tempfile.TemporaryDirectory() as work:
            header, parser = Path(work) / 'api.h', Path(work) / 'parser.c'
            header.write_text('#define TREE_SITTER_LANGUAGE_VERSION 15\n'
                              '#define TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION 13\n')
            for abi in (13, 14, 15):
                parser.write_text(f'#define LANGUAGE_VERSION {abi}\n')
                checked_in = identity(header, parser)
                self.assertEqual(checked_in['abi'], abi)
                self.assertIsNone(checked_in['generator_version'])
                for producer in ('tree-sitter 0.24.7', 'tree-sitter 99.0.0'):
                    generated = identity(header, parser, producer)
                    self.assertEqual(generated['generator_version'], producer)
                    self.assertEqual(generated['parser_sha256'], checked_in['parser_sha256'])
            for abi in (12, 16):
                parser.write_text(f'#define LANGUAGE_VERSION {abi}\n')
                with self.assertRaises(ValueError):
                    identity(header, parser)


if __name__ == '__main__':
    unittest.main()
