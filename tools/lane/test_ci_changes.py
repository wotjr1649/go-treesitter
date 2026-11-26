import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ci_changes as cc

ROOT = Path(__file__).resolve().parents[2]


def flags(workflow, files, event="pull_request"):
    return cc.decide(workflow, files, event)


def on(workflow, files, event="pull_request"):
    return sorted(k for k, v in flags(workflow, files, event).items() if v == "true" and k != "full")


class ClassifyTest(unittest.TestCase):
    def test_classes(self):
        table = {
            "README.md": "docs", "docs/specs/validation.md": "docs", "docs/img/a.png": "docs", "artifacts/x/y.json": "docs",
            "tools/gen/README.md": "docs", "AGENTS.md": "docs",
            "LICENSE": "license", "LICENSES/manifest.json": "license", "THIRD_PARTY_NOTICES.md": "license",
            "grammars/c/LICENSE": "license", "grammars/c/README.md": "grammars", "grammars/c/parser.go": "grammars",
            "go.mod": "root", "query.go": "root", "node_test.go": "root", "internal/engine/engine.go": "root",
            "experimental/match/match.go": "root", "testdata/newtonsoft/LICENSE.md": "root",
            "tools/gen/pins.json": "gen", "tools/gen/patches/yaml/a.patch": "gen",
            "tools/conformance/known_gaps_test.go": "conf", "tools/conformance/testdata/x.md": "conf",
            "tools/native-ref/main.go": "nativeref", "tools/bench/ci/main.go": "bench", "tools/lane/record.py": "lane",
            "tools/lane/ci_changes.py": "lib", "tools/apiguard/go.sum": "apiguard", "tools/compare/README.md": "docs",
            "tools/compare/adapters/x/go.mod": "tools-other", ".github/workflows/engine.yml": "wf:engine",
            ".github/workflows/bench.yml": "wf:other", ".github/CODEOWNERS": "unknown", ".gitignore": "unknown",
            "Makefile": "unknown", "tools/newtool/main.go": "unknown",
        }
        for path, want in table.items():
            self.assertEqual(cc.classify(path), want, path)


class DecideTest(unittest.TestCase):
    def test_docs_only_keeps_one_cheap_core(self):
        f = ["README.md", "docs/specs/validation.md"]
        self.assertEqual(on("engine", f), ["engine_core"])
        self.assertEqual(on("regen", f), [])
        self.assertEqual(on("windows", f), [])
        self.assertEqual(on("api-guard", f), [])
        self.assertEqual(on("pin-move", f), [])
        out = flags("conformance", f)
        self.assertEqual(out["conformance_full"], "false")
        self.assertEqual([(e["runner"], e["group"]["name"]) for e in json.loads(out["conformance_matrix"])["include"]],
                         [("ubuntu-24.04", "g1")])

    def test_root_code(self):
        f = ["query.go", "internal/engine/x.go"]
        self.assertEqual(on("engine", f), ["engine_compat", "engine_core", "engine_floor", "engine_grammars", "engine_platforms"])
        self.assertEqual(on("regen", f), ["regen"])
        self.assertEqual(on("windows", f), ["windows_product", "windows_race"])
        self.assertEqual(on("api-guard", f), ["api_guard"])
        self.assertEqual(on("pin-move", f), [])
        out = flags("conformance", f)
        self.assertEqual(len(json.loads(out["conformance_matrix"])["include"]), 10)
        self.assertEqual(out["full"], "false")  # PR: reduced engine platform matrix
        self.assertEqual(len(json.loads(flags("engine", f)["floor_shards"])["include"]), 1)

    def test_grammar_and_generator(self):
        self.assertEqual(on("engine", ["grammars/c/parser.go"]),
                         ["engine_compat", "engine_core", "engine_floor", "engine_grammars", "engine_platforms"])
        self.assertEqual(on("regen", ["grammars/c/parser.go"]), ["regen"])
        self.assertEqual(on("windows", ["grammars/c/parser.go"]), ["windows_licenses"])
        gen = ["tools/gen/pins.json", "tools/gen/regen.sh"]
        self.assertEqual(on("engine", gen), ["engine_compat", "engine_core", "engine_platforms"])  # no grammar shards: drift is regen's
        self.assertEqual(on("regen", gen), ["regen"])
        self.assertEqual(on("pin-move", gen), ["pin_move"])
        self.assertEqual(on("windows", gen), ["windows_licenses"])
        self.assertEqual(on("api-guard", gen), [])

    def test_ci_only(self):
        self.assertEqual(on("engine", [".github/workflows/bench.yml"]), [])
        self.assertEqual(on("regen", [".github/workflows/bench.yml"]), [])
        self.assertEqual(flags("conformance", [".github/workflows/bench.yml"])["conformance_full"], "false")
        # A change to the workflow itself runs all of that workflow (and only that one).
        eng = [".github/workflows/engine.yml"]
        self.assertEqual(flags("engine", eng)["full"], "true")
        self.assertEqual(len(json.loads(flags("engine", eng)["floor_shards"])["include"]), 5)
        self.assertEqual(len(on("engine", eng)), 5)
        self.assertEqual(on("regen", eng), [])
        reg = [".github/workflows/regen.yml"]
        self.assertEqual(on("regen", reg), ["regen"])
        self.assertEqual(on("pin-move", [".github/workflows/pin-move.yml"]), ["pin_move"])
        self.assertEqual(flags("conformance", [".github/workflows/conformance.yml"])["conformance_full"], "true")

    def test_conformance_inputs(self):
        for f in (["tools/conformance/a_test.go"], ["tools/native-ref/main.go"], ["tools/gen/pins.json"], ["grammars/c/parser.go"]):
            self.assertEqual(flags("conformance", f)["conformance_full"], "true", f)
        for f in (["tools/lane/record.py"], ["tools/compare/cmd/x.go"], ["docs/a.md"], ["tools/bench/ci/a.go"][:0] or ["LICENSE"]):
            self.assertEqual(flags("conformance", f)["conformance_full"], "false", f)

    def test_everything_runs(self):
        for f, ev in ((None, "pull_request"), ([], "pull_request"), (["Makefile"], "pull_request"),
                      (["README.md", "weird/file"], "pull_request"), (["tools/lane/ci_changes.py"], "pull_request"),
                      (["README.md"], "push"), (["README.md"], "workflow_dispatch"), (["README.md"], "schedule")):
            for wf in cc.WORKFLOWS:
                out = flags(wf, f, ev)
                self.assertTrue(all(v == "true" for k, v in out.items() if k[:3] in ("eng", "reg", "win", "api", "pin", "con") and v in ("true", "false")), (wf, f, ev, out))
            self.assertEqual(flags("engine", f, ev)["full"], "true" if ev != "pull_request" else "false")
        self.assertEqual(len(json.loads(flags("conformance", ["README.md"], "push")["conformance_matrix"])["include"]), 10)
        self.assertEqual(len(json.loads(flags("engine", ["README.md"], "push")["floor_shards"])["include"]), 5)

    def test_every_job_flag_is_covered(self):
        got = set()
        for wf in cc.WORKFLOWS:
            got |= {k for k in flags(wf, ["Makefile"]) if k in cc.NEEDS}
        self.assertEqual(got, set(cc.NEEDS))

    def test_unknown_class_is_never_skipped(self):
        # A path matched by no rule falls into "unknown", which runs everything.
        self.assertEqual(cc.classify("brand-new-top-dir/x"), "unknown")


class RepoTest(unittest.TestCase):
    def test_partitions_name_every_grammar_once(self):
        cc.partition_ok(ROOT)

    def test_partition_rejects_a_missing_grammar(self):
        with tempfile.TemporaryDirectory() as d:
            (Path(d) / "grammars" / "c").mkdir(parents=True)
            with self.assertRaises(SystemExit):
                cc.partition_ok(d)

    def test_cli_reads_files_from_stdin_and_writes_outputs(self):
        with tempfile.TemporaryDirectory() as d:
            out = Path(d, "out")
            res = subprocess.run([sys.executable, "-I", str(Path(cc.__file__)), "--workflow", "regen", "--files", "-",
                                  "--event", "pull_request", "--root", str(ROOT)], input="README.md\n", text=True,
                                 capture_output=True, env={"GITHUB_OUTPUT": str(out), "PATH": os.environ.get("PATH", ""),
                                                           "SYSTEMROOT": os.environ.get("SYSTEMROOT", "")})
            self.assertEqual(res.returncode, 0, res.stderr)
            self.assertIn("regen=false\n", out.read_text())


# The gate step shared by every PR-required workflow. It must appear verbatim in each of them (windows.yml is JSON, the others YAML;
# the command has no double quote or backslash, so it is the same text in both).
GATE_CMD = "jq -c 'map_values(.result)' <<<$NEEDS; jq -e --arg ok success --arg sk skipped " \
           "'all(.[]; .result == $ok or .result == $sk) and .changes.result == $ok' <<<$NEEDS"
GATE_WORKFLOWS = {"engine": "engine-gate", "regen": "regen-gate", "conformance": "conformance-gate", "windows": "windows-gate",
                  "api-guard": "api-guard-gate", "pin-move": "pin-move-gate"}


class GateTest(unittest.TestCase):
    def test_every_required_workflow_has_the_gate_and_a_changes_job(self):
        for wf, gate in GATE_WORKFLOWS.items():
            text = (ROOT / ".github" / "workflows" / (wf + ".yml")).read_text(encoding="utf-8")
            self.assertIn(GATE_CMD, text, wf)
            self.assertIn("name: %s\n" % gate if wf != "windows" else '"name": "%s"' % gate, text, wf)
            self.assertIn("--workflow %s" % wf, text, wf)
            self.assertEqual(text.count(GATE_CMD), 1, wf)

    def run_cmd(self, cmd, needs):
        import shutil
        bash, jq = shutil.which("bash"), shutil.which("jq")
        if not bash or not jq:
            self.skipTest("bash and jq are needed to run the gate step")
        env = dict(os.environ, NEEDS=json.dumps({k: {"result": v, "outputs": {}} for k, v in needs.items()}))
        return subprocess.run([bash, "-c", cmd], env=env, capture_output=True, text=True).returncode

    def test_gate_logic(self):
        cases = [
            ({"changes": "success", "a": "success", "b": "success"}, 0),
            ({"changes": "success", "a": "success", "b": "skipped"}, 0),  # intentionally skipped counts as passing
            ({"changes": "success", "a": "skipped", "b": "skipped"}, 0),
            ({"changes": "success", "a": "failure", "b": "success"}, 1),
            ({"changes": "success", "a": "cancelled", "b": "success"}, 1),
            ({"changes": "failure", "a": "skipped", "b": "skipped"}, 1),  # classifier failed: everything skipped, still red
            ({"changes": "cancelled", "a": "skipped"}, 1),
            ({"changes": "skipped", "a": "success"}, 1),
            ({"a": "success"}, 1),  # no changes job in needs
            ({"changes": "success", "a": "failure", "b": "skipped"}, 1),
        ]
        for needs, want in cases:
            self.assertEqual(self.run_cmd(GATE_CMD, needs), want, needs)


if __name__ == "__main__":
    unittest.main()
