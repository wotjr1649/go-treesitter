import importlib.util
from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
MODULE = Path(__file__).with_name('separate.py')
SPEC = importlib.util.spec_from_file_location('runtime_bundle_separate', MODULE)
separate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(separate)


def registry_entry(name):
    return f'\tRegister(LangEntry{{\n\t\tName: "{name}",\n\t\t// fixture\n\t}})\n'.encode()


def language_function(name):
    return f'// {name}Language\nfunc {name}Language() *gotreesitter.Language {{\n return nil\n}}\n\n'.encode()


class OptionalScannerBindingsTest(unittest.TestCase):
    def test_separate_reuses_main_scanner_registry_and_copies_binding(self):
        helper_path = ROOT / 'internal/runtime/grammars/runtime/external_scanner_binding.go'
        helper = helper_path.read_bytes()
        files = {
            'grammars/runtime/external_scanner_binding.go': helper,
            'grammars/registry_builtin_gen.go': b''.join(
                registry_entry(name) for name in ('caddy', 'disassembly', 'jq', 'brightscript', 'cooklang')),
            'grammars/embedded_grammars_gen.go': b''.join(
                language_function(name) for name in ('Caddy', 'Disassembly', 'Jq', 'Brightscript', 'Cooklang')),
            'grammars/runtime/builtin_scanners_gen.go': (
                b'\tRegisterCaddySupport()\n\tRegisterDisassemblySupport()\n\tRegisterCooklangSupport()\n'),
            'grammars/blob_source_embedded_core.go': b''.join(
                f'//go:embed grammar_blobs/{name}.bin\n'.encode()
                for name in ('caddy', 'disassembly', 'jq', 'brightscript', 'cooklang')),
        }
        for name in ('caddy', 'disassembly', 'jq'):
            files[f'grammars/grammar_blobs/{name}.bin'] = b'grammar'
        for name in ('caddy', 'disassembly'):
            files[f'grammars/runtime/{name}_scanner.go'] = b'package grammarruntime\n'
            files[f'grammars/runtime/{name}_external_lex_states_gen.go'] = b'package grammarruntime\n'
        for i in range(12):
            files[f'grammars/runtime/support_{i}_caddy.go'] = b'package grammarruntime\n'
        for name in ('caddy', 'disassembly', 'jq', 'caddy_extra'):
            files[f'grammars/runtime/{name}_no_copyleft_stub.go'] = b'stub'
        excluded = (
            'grammars/grammar_blobs/brightscript.bin', 'grammars/runtime/brightscript_scanner.go',
            'grammars/brightscript_a.dat', 'grammars/brightscript_b.dat', 'grammars/brightscript_c.dat',
            'grammars/grammar_blobs/cooklang.bin', 'grammars/runtime/cooklang_scanner.go',
            'grammars/cooklang_a.dat', 'grammars/cooklang_b.dat', 'grammars/cooklang_c.dat',
            'grammars/cooklang_d.dat',
        )
        files.update({path: b'excluded' for path in excluded})

        main, optional = separate.separate(files, 'example.com/runtime')

        self.assertEqual(len(optional), 9)
        self.assertEqual(len(files) - len(main), 34)
        self.assertEqual(optional['external_scanner_binding.go'],
                         helper.replace(b'package grammarruntime', b'package gtsadapter', 1))
        self.assertNotIn('external_scanner_spec.go', optional)
        self.assertIn(b'type ExternalScannerSpec = grammarruntime.ExternalScannerSpec', optional['catalog.go'])
        self.assertIn(b'type ExternalScannerSourceFile = grammarruntime.ExternalScannerSourceFile', optional['catalog.go'])
        self.assertIn(b'var RegisterExternalScannerSpec = grammarruntime.RegisterExternalScannerSpec', optional['catalog.go'])
        self.assertEqual(main['grammars/runtime/external_scanner_binding.go'], helper)

    def test_rejects_missing_or_unexpected_package_source(self):
        with self.assertRaisesRegex(ValueError, 'missing optional scanner binding source'):
            separate.optional_scanner_binding({})
        with self.assertRaisesRegex(ValueError, 'optional scanner binding package drift'):
            separate.optional_scanner_binding({'grammars/runtime/external_scanner_binding.go': b'package other\n'})


if __name__ == '__main__':
    unittest.main()
