import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SUFFIX = re.compile(r"_(windows|linux|darwin|freebsd|unix|js|wasip1|amd64|arm64|arm|386)(_test)?\.go$")


class ConformanceVetIsPlatformIndependentTest(unittest.TestCase):
    """engine.yml vets tools/conformance on ubuntu-24.04 only. That is sound while the module has no platform-specific code."""

    def test_no_platform_specific_code(self):
        files = sorted((ROOT / "tools/conformance").rglob("*.go"))
        self.assertTrue(files)
        for f in files:
            rel = f.relative_to(ROOT).as_posix()
            self.assertIsNone(SUFFIX.search(f.name), rel + ": platform file name; vet it on every platform again (engine.yml)")
            text = f.read_text(encoding="utf-8")
            for constraint in re.findall(r"^//go:build (.*)$", text, flags=re.M):
                self.assertRegex(constraint, r"^!no_[a-z0-9]+$", rel + ": platform build constraint")
            for bad in ('import "C"', '"syscall"', "golang.org/x/sys"):
                self.assertNotIn(bad, text, rel)

    def test_workflow_restricts_the_step_to_one_platform(self):
        text = (ROOT / ".github/workflows/engine.yml").read_text(encoding="utf-8")
        step = text[text.index("- name: Conformance module (vet only)"):].split("\n      - ")[0]
        self.assertIn("env.EXPECTED_GOOS == 'linux'", step)
        self.assertIn("env.EXPECTED_GOARCH == 'amd64'", step)


if __name__ == "__main__":
    unittest.main()
