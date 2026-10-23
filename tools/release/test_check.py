import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import check


class GraphBoundaryTest(unittest.TestCase):
    def test_graph_rejects_replace_extra_and_wrong_version(self):
        rows = [{'Path': 'example.com/release-consumer', 'Main': True},
                {'Path': check.MODULE, 'Version': check.VERSION}]
        encode = lambda values: '\n'.join(json.dumps(value) for value in values)
        self.assertEqual(len(check.check_graph(encode(rows), False)), 2)
        for change in ({'Replace': {'Path': '../local'}}, {'Version': 'v9.0.0'}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                check.check_graph(encode([rows[0], dict(rows[1], **change)]), False)
        with self.assertRaises(ValueError):
            check.check_graph(encode(rows + [{'Path': 'example.com/unexpected'}]), False)
        rows.append({'Path': check.MODULE + '/grammars/gpl', 'Version': check.VERSION})
        self.assertEqual(len(check.check_graph(encode(rows), True)), 3)


class CheckoutTest(unittest.TestCase):
    def test_linked_worktree_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / 'repo'
            git = lambda *args, cwd=root: subprocess.run(['git', *args], cwd=cwd, check=True, capture_output=True,
                                                         env=check.base_env())
            git('init', '-q', str(root), cwd=directory)
            git('-c', 'user.name=t', '-c', 'user.email=t@example.invalid', '-c', 'commit.gpgsign=false',
                'commit', '-q', '--allow-empty', '-m', 'x')
            git('worktree', 'add', '-q', str(Path(directory) / 'linked'))
            self.assertTrue(check.regular_checkout(root))
            self.assertFalse(check.regular_checkout(Path(directory) / 'linked'))


if __name__ == '__main__':
    unittest.main()
