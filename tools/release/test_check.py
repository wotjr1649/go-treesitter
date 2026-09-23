import json
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


if __name__ == '__main__':
    unittest.main()
