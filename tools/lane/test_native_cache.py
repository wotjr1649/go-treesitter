import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MODULE = "github.com/wotjr1649/go-treesitter/"


def local_imports(directory):
    """Repository directories imported by the non-test Go files of directory (not recursive)."""
    found = set()
    for f in directory.glob("*.go"):
        if not f.name.endswith("_test.go"):
            found |= set(re.findall(r'"%s([^"]+)"' % re.escape(MODULE), f.read_text(encoding="utf-8")))
    return found


class NativeCacheKeyTest(unittest.TestCase):
    """The cache key of conformance.yml must name every repository directory the cached native build reads."""

    def test_key_covers_what_native_ref_imports(self):
        text = (ROOT / ".github/workflows/conformance.yml").read_text(encoding="utf-8")
        key = re.search(r"key: (conformance-native-.*)", text).group(1)
        patterns = re.findall(r"'([^']+)'", key)
        todo, seen = ["tools/native-ref"], set()
        while todo:
            d = todo.pop()
            if d in seen:
                continue
            seen.add(d)
            todo += sorted(local_imports(ROOT / d))
        for d in seen:
            self.assertTrue(any(d == p[:-3] or d.startswith(p[:-3] + "/") for p in patterns if p.endswith("/**"))
                            or any(p.startswith(d + "/") for p in patterns), "cache key does not cover " + d)
        # data the build reads that is not a Go import
        for needed in ("tools/gen/pins.json", "tools/gen/patches/**", "tools/native-ref/**", ".github/workflows/conformance.yml"):
            self.assertIn(needed, patterns)


class CacheSumsTest(unittest.TestCase):
    def setUp(self):
        self.bash = shutil.which("bash")
        if not self.bash or not shutil.which("sha256sum"):
            if sys.platform.startswith("linux"):
                self.fail("bash and sha256sum are needed")
            self.skipTest("bash and sha256sum are needed")

    def run_sums(self, mode, d):
        return subprocess.run([self.bash, str(ROOT / "tools/native-ref/cachesums.sh"), mode, Path(d).as_posix()],
                              capture_output=True, text=True).returncode

    def test_round_trip_and_damage(self):
        with tempfile.TemporaryDirectory() as d:
            t = Path(d)
            (t / "exe").mkdir()
            (t / "exe" / "c").write_bytes(b"binary")
            (t / "scanner-stages").mkdir()
            (t / "scanner-stages" / "c.json").write_text("{}")
            self.assertEqual(self.run_sums("write", t), 0)
            self.assertEqual(self.run_sums("check", t), 0)
            (t / "exe" / "extra").write_bytes(b"x")
            self.assertNotEqual(self.run_sums("check", t), 0)  # a file that was not recorded
            (t / "exe" / "extra").unlink()
            (t / "exe" / "c").write_bytes(b"other")
            self.assertNotEqual(self.run_sums("check", t), 0)  # changed bytes
            (t / "exe" / "c").unlink()
            self.assertNotEqual(self.run_sums("check", t), 0)  # a recorded file is gone
            self.assertNotEqual(self.run_sums("write", d + "/missing"), 0)

    def test_record_from_another_temp_dir_is_refused(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            (Path(a) / "exe").mkdir()
            self.assertEqual(self.run_sums("write", a), 0)
            shutil.copy(Path(a) / "native-cache.sums", Path(b) / "native-cache.sums")
            self.assertNotEqual(self.run_sums("check", b), 0)


if __name__ == "__main__":
    unittest.main()
