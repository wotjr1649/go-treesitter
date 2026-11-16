import csv
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import tarfile
import unittest
from unittest.mock import Mock, patch
from argparse import Namespace

spec = importlib.util.spec_from_file_location("generatorgate", Path(__file__).with_name("generatorgate.py"))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class GeneratorGateTest(unittest.TestCase):
    def test_text_identity_is_lf_canonical_but_binary_identity_is_exact(self):
        with tempfile.TemporaryDirectory() as tmp:
            lf, crlf = Path(tmp) / "lf", Path(tmp) / "crlf"
            lf.write_bytes(b"alpha\nbeta\n")
            crlf.write_bytes(b"alpha\r\nbeta\r\n")
            self.assertEqual(gate.text_sha(lf), gate.text_sha(crlf))
            self.assertNotEqual(gate.sha(lf), gate.sha(crlf))

    def test_lfs_sharing_requires_oid_and_all_c_headers_and_no_patches(self):
        with tempfile.TemporaryDirectory() as tmp:
            original, candidate = Path(tmp) / "original", Path(tmp) / "candidate"
            original.mkdir()
            candidate.mkdir()
            (candidate / "parser.c").write_bytes(b"generated C")
            oid = gate.sha(candidate / "parser.c")
            pointer = f"version https://git-lfs.github.com/spec/v1\noid sha256:{oid}\nsize 11\n"
            (original / "parser.c").write_bytes(pointer.encode())
            for root in (original, candidate):
                (root / "scanner.c").write_bytes(b"scanner")
                (root / "parser.h").write_bytes(b"header")
            self.assertEqual(gate.identical_lfs_reference(original, candidate, {}), oid)
            self.assertIsNone(gate.identical_lfs_reference(original, candidate, {"patches": [{}]}))
            for name in ("scanner.c", "parser.h", "parser.c"):
                path = candidate / name
                before = path.read_bytes()
                path.write_bytes(before + b"changed")
                self.assertIsNone(gate.identical_lfs_reference(original, candidate, {}))
                path.write_bytes(before)
            (candidate / "extra.h").write_bytes(b"new input")
            self.assertIsNone(gate.identical_lfs_reference(original, candidate, {}))
            (original / "parser.c").write_bytes(b"version https://git-lfs.github.com/spec/v1\n")
            with self.assertRaisesRegex(ValueError, "no sha256 OID"):
                gate.identical_lfs_reference(original, candidate, {})

    def test_interrupt_and_other_wait_exceptions_stop_child_tree(self):
        with tempfile.TemporaryDirectory() as tmp:
            for error in (KeyboardInterrupt(), OSError("wait failed")):
                proc = Mock()
                proc.wait.side_effect = error
                with patch.object(gate.subprocess, "Popen", return_value=proc), patch.object(gate, "stop_tree") as stop:
                    with self.assertRaises(type(error)):
                        gate.execute(["child"], tmp, Path(tmp) / "run.log")
                    stop.assert_called_once_with(proc)

    def test_timeout_stops_child_and_retains_failure_receipt(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "timeout.log"
            with self.assertRaisesRegex(RuntimeError, "exited 124"):
                gate.execute([sys.executable, "-c", "import time; time.sleep(60)"], tmp, log, timeout=0.1)
            receipt = json.loads(log.with_suffix(".command.json").read_text())
            self.assertEqual(receipt["exit"], 124)
            self.assertEqual(receipt["timeoutSeconds"], 0.1)

    def test_windows_taskkill_and_followup_wait_are_bounded(self):
        proc = Mock(pid=123)
        with patch.object(gate.os, "name", "nt"), patch.object(gate.subprocess, "run") as run:
            run.return_value.returncode = 0
            gate.stop_tree(proc)
            self.assertEqual(run.call_args.kwargs["timeout"], 10)
            proc.wait.assert_called_once_with(timeout=10)
        proc.reset_mock()
        with patch.object(gate.os, "name", "nt"), patch.object(gate.subprocess, "run") as run:
            run.side_effect = subprocess.TimeoutExpired("taskkill", 10)
            with self.assertRaisesRegex(RuntimeError, "taskkill did not finish"):
                gate.stop_tree(proc)
            proc.kill.assert_called_once()
            proc.wait.assert_called_once_with(timeout=10)

    def test_assertion_success_is_not_an_unreported_cli_failure(self):
        passed = {"AssertionPassed": {"assertion_count": 13}}
        failed = {"AssertionFailed": {"error": "new error"}}
        for value in ("Passed", "Skipped", "Platform", passed):
            self.assertTrue(gate.is_passed(value))
        for value in ("Failed", "Updated", failed, {"unknown": {}}, {"AssertionPassed": {}, "AssertionFailed": {}}):
            self.assertFalse(gate.is_passed(value))
        before = {"parse_results": [], "highlight_results": [{"name": "case", "outcome": passed}]}
        after = {"parse_results": [], "highlight_results": [{"name": "case", "outcome": failed}]}
        self.assertEqual(gate.compare_upstream(before, after)[0]["id"], "upstream/highlight_results/case")
        self.assertTrue(all(gate.is_passed(value) for value in gate.outcomes(before).values()))

    def test_large_concat_uses_upstream_only_input_projection(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "tools/bench/ci").mkdir(parents=True)
            gate.save(root / "tools/bench/ci/large.json", [{"lang": "tsql", "kind": "concat"}])
            args = Namespace(lang="tsql", ciconf="ciconf")
            with patch.object(gate, "execute") as execute:
                data = gate.large_input_data(args, root, root, root / "pins.json", "tsql", {})
            command = execute.call_args.args[0]
            self.assertIn("-inputs-only", command)
            self.assertNotIn("-kit", command)
            self.assertEqual(data, root / "large-input-data")
            args.lang = "json"
            with patch.object(gate, "execute") as execute:
                self.assertEqual(gate.large_input_data(args, root, root, root / "pins.json", "json", {}), root / "reference/data")
                execute.assert_not_called()

    def test_upstream_tests_share_verified_candidate_dependencies(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            prepared = root / "candidate/work/patches-lang"
            module = prepared / "first/nested/node_modules/dependency"
            module.mkdir(parents=True)
            (module / "query.scm").write_text("query")
            upstream = root / "upstream"
            upstream.mkdir()
            (upstream / "package-lock.json").write_text("lock")
            provenance = {"npm": [{"name": "dependency", "scriptsExecuted": False,
                                   "lockfileSHA256": gate.sha(upstream / "package-lock.json")} ]}
            gate.save(prepared / "provenance.json", provenance)
            grammar = {"grammarDir": "nested/src", "npmDependencies": ["dependency"]}
            report = {"grammar": "lang"}
            gate.upstream_dependencies(root, upstream, grammar, "reference", report)
            self.assertEqual((upstream / "node_modules/dependency/query.scm").read_text(), "query")
            self.assertEqual(report["referenceUpstreamNPM"], provenance["npm"])
            (upstream / "package-lock.json").write_text("changed")
            with self.assertRaisesRegex(ValueError, "provenance mismatch"):
                gate.upstream_dependencies(root, upstream, grammar, "candidate", report)

    def test_exception_disappearance_and_expansion_fail(self):
        expected = ["corpus/lang/case", "upstream/parse_results/suite/case"]
        failures = [{"id": case} for case in expected]
        self.assertTrue(gate.exception_matches(expected, failures))
        self.assertFalse(gate.exception_matches(expected, []))
        self.assertFalse(gate.exception_matches(expected, failures + [{"id": "corpus/lang/new"}]))
        self.assertFalse(gate.exception_matches([], []))

    def test_supplement_recovers_auxiliary_cases_without_erasing_corpus_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "candidate/upstream/test/corpus").mkdir(parents=True)
            original = {"parse_results": [{"name": "suite", "children": [{"name": "case.+", "outcome": "Failed"}]}],
                        "parse_failures": [{"name": "case.+", "actual": "bad", "expected": "good"}],
                        "parse_stats": {"total_parses": 1}, "highlight_results": []}
            extra = {"parse_results": [], "parse_stats": {"total_parses": 0},
                     "highlight_results": [{"name": "highlight", "outcome": "Passed"}]}
            report = {"candidateUpstream": original}

            def execute(command, cwd, log, env, **kwargs):
                self.assertEqual(command[-1], r"^(?:case\.\+)$")
                self.assertEqual(env["TREE_SITTER_LIBDIR"], str(root / "candidate/lib"))
                log.write_text(json.dumps(extra))
                return 0

            with patch.object(gate, "execute", execute):
                gate.complete_upstream(Namespace(cli="cli"), root, root, "candidate", {"grammarDir": "src"}, report, {})
            combined = report["candidateUpstream"]
            self.assertEqual(combined["parse_failures"], original["parse_failures"])
            self.assertEqual(combined["parse_results"], original["parse_results"])
            self.assertEqual(report["candidateUpstreamCoverage"], 2)
            self.assertEqual(report["candidateUpstreamFullSummary"], original)

    def test_archive_hash_and_escape_are_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            archive = root / "archive.tar.gz"
            with tarfile.open(archive, "w:gz") as tar:
                entry = tarfile.TarInfo("top/C:/escape")
                entry.size = 1
                tar.addfile(entry, io.BytesIO(b"x"))
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                gate.extract(archive, root / "bad-hash", "0" * 64)
            self.assertFalse((root / "bad-hash").exists())
            with self.assertRaisesRegex(ValueError, "unsafe archive"):
                gate.extract(archive, root / "bad-path", gate.sha(archive))

    def test_upstream_preserves_existing_failure_signature(self):
        before = {"parse_results": [{"name": "suite", "children": [{"name": "case", "outcome": "Failed"}]}],
                  "parse_failures": [{"name": "case", "actual": "old", "expected": "expected"}],
                  "parse_stats": {"total_parses": 1}}
        after = json.loads(json.dumps(before))
        self.assertEqual(gate.compare_upstream(before, after), [])
        after["parse_failures"][0]["actual"] = "different"
        self.assertEqual(gate.compare_upstream(before, after)[0]["id"], "upstream/parse_failures/case")
        after["parse_results"] = []
        with self.assertRaisesRegex(ValueError, "cases changed"):
            gate.compare_upstream(before, after)

    def test_upstream_new_failure_and_zero_cases(self):
        before = {"parse_results": [{"name": "suite", "children": [{"name": "case", "outcome": "Passed"}]}]}
        after = json.loads(json.dumps(before))
        after["parse_results"][0]["children"][0]["outcome"] = "Failed"
        self.assertEqual(gate.compare_upstream(before, after)[0]["id"], "upstream/parse_results/suite/case")
        self.assertEqual(gate.compare_upstream({"parse_results": []}, {"parse_results": []}), [])

    def test_summary_retains_warning_prefix_and_rejects_missing_summary(self):
        value = {"parse_results": [], "parse_stats": {"total_parses": 0}}
        self.assertEqual(gate.summary("warning {not-json}\n" + json.dumps(value)), value)
        with self.assertRaisesRegex(ValueError, "summary missing"):
            gate.summary('{"parse_results": []}')

    def test_native_timing_is_ignored_but_tree_and_ranges_changes_fail(self):
        with tempfile.TemporaryDirectory() as tmp:
            roots = [Path(tmp) / side for side in ("before", "after")]
            for i, root in enumerate(roots):
                (root / "corpus").mkdir(parents=True)
                (root / "edits/lang").mkdir(parents=True)
                (root / "native/dumps/lang").mkdir(parents=True)
                for rel, header, data in [
                    ("corpus/manifest.csv", ["id", "sha256"], ["lang/case", "input-hash"]),
                    ("edits/manifest.csv", ["id", "old_sha256", "new_sha256"], ["lang/case-e1", "old", "new"]),
                    ("edits/results-native.csv", ["id", "edit_sha256", "edit_ms"], ["lang/case-e1", "tree", str(i)]),
                ]:
                    with (root / rel).open("w", newline="") as stream:
                        csv.writer(stream).writerows([header, data])
                for rel in ("native/dumps/lang/case.dump", "edits/lang/case-e1.ranges", "edits/lang/case-e1.fields"):
                    (root / rel).write_text("same")
            self.assertEqual(gate.compare_data(*roots), [])
            (roots[1] / "edits/lang/case-e1.ranges").write_text("changed")
            differences = gate.compare_data(*roots)
            self.assertEqual(differences[0]["id"], "edits/lang/case-e1")
            self.assertEqual(differences[0]["changed"], ["ranges"])
            (roots[1] / "native/dumps/lang/case.dump").write_text("changed tree")
            self.assertEqual(len(gate.compare_data(*roots)), 2)
            (roots[1] / "edits/results-native.csv").write_text("id,edit_sha256,edit_ms\n")
            with self.assertRaisesRegex(ValueError, "cover the input manifest"):
                gate.compare_data(*roots)

    def test_empty_native_coverage_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            roots = [Path(tmp) / side for side in ("before", "after")]
            for root in roots:
                (root / "corpus").mkdir(parents=True)
                (root / "edits").mkdir()
                (root / "corpus/manifest.csv").write_text("id,sha256\n")
                (root / "edits/manifest.csv").write_text("id,old_sha256,new_sha256\n")
            with self.assertRaisesRegex(ValueError, "must both be nonempty"):
                gate.compare_data(*roots)


if __name__ == "__main__":
    unittest.main()
