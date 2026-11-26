import unittest

import consumer_compat
import published_consumers


class VersionTest(unittest.TestCase):
    def test_semver_matches_go(self):
        for ok in ("v0.1.0", "v1.0.0-rc.1", "v1.0.0-rc.10", "v1.0.0-0a.b-c.1", "v1.0.0-RC.1"):
            self.assertTrue(consumer_compat.SEMVER.fullmatch(ok), ok)
        for bad in ("", "0.1.0", "v0.1", "v01.0.0", "v1.0.0-", "v1.0.0-.", "v1.0.0-rc..1", "v1.0.0-rc.1.",
                    "v1.0.0-rc.01", "v1.0.0-rc.1+meta", "v1.0.0-rc.1\n"):
            self.assertFalse(consumer_compat.SEMVER.fullmatch(bad), bad)

    def test_published_consumers_rejects_pseudo_and_invalid(self):
        self.assertEqual(published_consumers.version("v1.0.0-rc.1"), "v1.0.0-rc.1")
        for bad in ("v1.0.0-rc.01", "v1.0.0-0.20260101000000-0123456789ab", "v1.0.0-rc.1.0.20260101000000-0123456789ab"):
            with self.assertRaises(Exception, msg=bad):
                published_consumers.version(bad)


class CandidateTest(unittest.TestCase):
    def test_candidate_separator(self):
        self.assertEqual(consumer_compat.candidate_version("v0.7.0", "abc"), "v0.7.0-consumercompat.abc")
        # A dot keeps the candidate between rc.1 and rc.2; a hyphen would sort it after rc.2.
        self.assertEqual(consumer_compat.candidate_version("v1.0.0-rc.1", "abc"), "v1.0.0-rc.1.consumercompat.abc")
        with self.assertRaises(ValueError):
            consumer_compat.candidate_version("v1.0.0-rc.01", "abc")

    def test_min_root_is_a_version(self):
        for row in consumer_compat.GRAMMARS.values():
            self.assertEqual(len(row), 5)
            self.assertTrue(consumer_compat.SEMVER.fullmatch(row[4]))


if __name__ == "__main__":
    unittest.main()
