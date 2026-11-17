import csv
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import tarfile
import unittest
from unittest.mock import Mock, patch
from argparse import Namespace

from proctree_test import assert_stopped, tree_command

spec = importlib.util.spec_from_file_location("generatorgate", Path(__file__).with_name("generatorgate.py"))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class GeneratorGateTest(unittest.TestCase):
    def test_patch_expectations_require_exact_input_and_result_signatures(self):
        import copy
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            grammar = {"commit": "rev", "archiveSHA256": "archive", "patches": [
                {"source": "grammar.js", "sha256": "patch"}]}
            pins = {"runtime": {"commit": "runtime"}, "tools": {"treeSitterCLI": "0.27.2"}}
            report = {"grammar": "tiny", "seed": 1, "corpusManifestSHA256": "corpus",
                      "editsManifestSHA256": "edits", "largeManifestSHA256": "large", "patchCorpusFilesSHA256": {}, "differences": [
                {"id": "corpus/tiny/korean", "kind": "corpus", "inputSHA256": "input",
                 "referenceDumpSHA256": "a" * 64, "candidateDumpSHA256": "b" * 64}]}
            self.assertFalse(gate.check_patch_expectations(repo, pins, grammar, report))
            self.assertTrue(gate.check_patch_expectations(repo, pins, grammar, report | {"differences": []}))
            identity = {"grammarCommit": "rev", "grammarArchiveSHA256": "archive",
                        "runtimeCommit": "runtime", "cli": "0.27.2", "seed": 1,
                        "patches": grammar["patches"], "corpusManifestSHA256": "corpus",
                        "editsManifestSHA256": "edits", "largeManifestSHA256": "large", "patchCorpusFilesSHA256": {}}
            expected = {"identity": identity, "reason": "recorded fix", "retireWhen": "upstream fix",
                        "differences": copy.deepcopy(report["differences"])}
            path = repo / "tools/gen/patches/tiny/generator-expectations.json"
            gate.save(path, expected)
            self.assertTrue(gate.check_patch_expectations(repo, pins, grammar, report))
            for key in ("id", "inputSHA256", "referenceDumpSHA256", "candidateDumpSHA256"):
                changed = copy.deepcopy(report)
                changed["differences"][0][key] = "c" * 64
                self.assertFalse(gate.check_patch_expectations(repo, pins, grammar, changed))
            for diffs in ([], report["differences"] * 2):
                changed = report | {"differences": diffs}
                self.assertFalse(gate.check_patch_expectations(repo, pins, grammar, changed))
            for key in identity:
                stale = copy.deepcopy(expected)
                stale["identity"][key] = None
                gate.save(path, stale)
                with self.assertRaisesRegex(ValueError, "identity"):
                    gate.check_patch_expectations(repo, pins, grammar, report)
            for diffs in (expected["differences"] * 2, [{"id": "case"}]):
                gate.save(path, expected | {"differences": diffs})
                with self.assertRaisesRegex(ValueError, "signatures"):
                    gate.check_patch_expectations(repo, pins, grammar, report)

    def test_strict_upstream_records_improvements_skips_and_changed_failure_trees(self):
        before = {"parse_results": [{"name": "case", "outcome": "Failed"}],
                  "parse_failures": [{"name": "case", "actual": "old"}]}
        for outcome in ("Passed", "Skipped", "Platform", "Failed"):
            after = {"parse_results": [{"name": "case", "outcome": outcome}],
                     "parse_failures": [{"name": "case", "actual": "new"}]}
            differences = gate.compare_upstream(before, after, strict=True)
            self.assertTrue(differences)
            self.assertTrue(all(len(x["referenceDumpSHA256"]) == 64 for x in differences))
            self.assertTrue(all(x["referenceDumpSHA256"] != x["candidateDumpSHA256"] for x in differences))
        self.assertEqual(gate.compare_upstream(before, before, strict=True), [])

    def test_strict_upstream_does_not_lose_duplicate_names(self):
        before = {"parse_results": [{"name": "case", "outcome": "Failed"}],
                  "parse_failures": [{"name": "case", "actual": "A"}, {"name": "case", "actual": "B"}]}
        after = json.loads(json.dumps(before))
        after["parse_failures"][0]["actual"] = "changed"
        self.assertEqual(len(gate.compare_upstream(before, after, strict=True)), 1)
        before["parse_results"] *= 2
        with self.assertRaisesRegex(ValueError, "duplicate upstream"):
            gate.compare_upstream(before, before, strict=True)

    def test_patch_corpus_is_additive_identical_and_cannot_overwrite_upstream(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            fixture = repo / "tools/gen/patches/lang/corpus/korean.txt"
            fixture.parent.mkdir(parents=True)
            fixture.write_bytes("SELECT 한글;\n".encode("utf-8"))
            grammar = {"extractDir": "upstream", "grammarDir": "lang/src"}
            for side in ("reference", "candidate"):
                corpus = repo / side
                target = corpus / "upstream/lang/test/corpus/grammar-patch-korean.txt"
                target.parent.mkdir(parents=True)
                self.assertEqual(gate.add_patch_corpus(repo, corpus, grammar, "lang"), {"korean.txt": gate.sha(fixture)})
                self.assertEqual(target.read_bytes(), fixture.read_bytes())
                with self.assertRaises(FileExistsError):
                    gate.add_patch_corpus(repo, corpus, grammar, "lang")
                self.assertEqual(target.read_bytes(), fixture.read_bytes())

    def test_reference_none_requires_successful_nonempty_upstream_tests(self):
        passed = {"parse_results": [{"name": "case", "outcome": "Passed"}],
                  "parse_stats": {"total_parses": 1}}
        report = {"differences": []}
        gate.check_upstream(0, passed, report)
        self.assertEqual(report["differences"], [])
        failed = json.loads(json.dumps(passed))
        failed["parse_results"][0]["outcome"] = "Failed"
        for code, result in ((1, passed), (0, failed),
                             (0, {"parse_results": [], "parse_stats": {"total_parses": 0}})):
            report = {"differences": []}
            gate.check_upstream(code, result, report)
            self.assertTrue(report["differences"])

    def test_swift_query_exception_ratchets_failure_and_checks_correct_query(self):
        import copy
        args = Namespace(lang="swift", cli="tree-sitter")
        pins = {"tools": {"treeSitterCLI": "0.27.2"}}
        grammar = {"upstreamTestException": {
            "cli": "0.27.2", "step": "tree-sitter test: queries/outline.scm -> test/outline/outline.swift",
            "reason": "owner approved", "retireWhen": "CLI pin move"}}
        result = {"parse_results": [{"name": str(n), "outcome": "Passed"} for n in range(312)],
                  "parse_stats": {"successful_parses": 312, "total_parses": 312}, "parse_failures": [],
                  "highlight_results": [{"name": str(n), "outcome": {"AssertionPassed": {}}} for n in range(8)],
                  "query_results": [{"name": "outline", "children": []}]}
        raw = "Error: Query compilation failed\n\nCaused by:\n    Query error at 1:1. Invalid syntax:\n    import Foundation\n    ^\n"
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def execute(command, cwd, log, env):
                self.assertEqual(command[:4], ["tree-sitter", "query", "queries/outline.scm", "test/outline/outline.swift"])
                log.write_text("captures")
            with patch.object(gate, "execute", execute):
                report = {}
                self.assertTrue(gate.swift_query_exception(args, pins, grammar, 1, result, raw, root, root, report, {}))
                self.assertTrue(report["upstreamTestExceptionMatches"])
                self.assertEqual(report["upstreamOutlineQueryExit"], 0)
            for label in ("fixed", "new-cli", "signature", "extra-error", "corpus", "skip", "highlight", "query"):
                changed = copy.deepcopy(result)
                changed_pins = copy.deepcopy(pins)
                code, log = 1, raw
                if label == "fixed": code = 0
                if label == "new-cli": changed_pins["tools"]["treeSitterCLI"] = "0.27.3"
                if label == "signature": log = raw.replace("Foundation", "NewFailure")
                if label == "extra-error": log += "new failure"
                if label == "corpus": changed["parse_failures"] = [{"name": "emoji"}]
                if label == "skip": changed["parse_results"][0]["outcome"] = "Skipped"
                if label == "highlight": changed["highlight_results"][0]["outcome"] = "Failed"
                if label == "query": changed["query_results"][0]["children"] = [{"name": "new", "outcome": "Failed"}]
                with self.subTest(label=label), patch.object(gate, "execute") as command:
                    with self.assertRaisesRegex(ValueError, "stale|signature"):
                        gate.swift_query_exception(args, changed_pins, grammar, code, changed, log, root, root, {}, {})
                    command.assert_not_called()
            with patch.object(gate, "execute", side_effect=RuntimeError("query failed")):
                with self.assertRaisesRegex(RuntimeError, "query failed"):
                    gate.swift_query_exception(args, pins, grammar, 1, result, raw, root, root, {}, {})

    def test_candidate_determinism_schema_and_product_lineage_are_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            prepared = root / "candidate/work/patches-swift"
            source = prepared / "first/src"
            module = root / "grammars/swift"
            source.mkdir(parents=True)
            module.mkdir(parents=True)
            (source / "parser.c").write_bytes(b"parser")
            (source / "node-types.json").write_bytes(b"[]")
            outputs = {name: gate.sha(source / name) for name in ("parser.c", "node-types.json")}
            provenance = {"deterministic": True, "outputs": outputs}
            gate.save(prepared / "provenance.json", provenance)
            for name in ("engine_gen.go", "engine_gen.dat"):
                (module / name).write_bytes(name.encode())
            product = {"grammarGeneration": provenance, "identity": {"GrammarRev": "rev"},
                       "outputs": {name: gate.sha(module / name) for name in ("engine_gen.go", "engine_gen.dat")}}
            gate.save(module / "provenance.json", product)
            args = Namespace(lang="swift")
            grammar = {"grammarDir": "src", "commit": "rev"}

            def execute(command, cwd, log, env):
                gate.save(log, {"assessment": "PASS", "input": {"sha256": outputs["node-types.json"]}})

            report = {}
            with patch.object(gate, "execute", execute):
                gate.check_candidate(args, root, root, grammar, report, {})
            self.assertEqual(report["candidateSchema"]["assessment"], "PASS")
            self.assertIn("engine_gen.dat", report["productModuleFilesSHA256"])
            with patch.object(gate, "execute", lambda command, cwd, log, env: gate.save(log, {"assessment": "FAIL"})):
                with self.assertRaisesRegex(ValueError, "strict schema"):
                    gate.check_candidate(args, root, root, grammar, {}, {})
            provenance["deterministic"] = False
            gate.save(prepared / "provenance.json", provenance)
            with self.assertRaisesRegex(ValueError, "determinism"):
                gate.check_candidate(args, root, root, grammar, {}, {})
            provenance["deterministic"] = True
            gate.save(prepared / "provenance.json", provenance)
            (source / "parser.c").write_bytes(b"changed")
            with self.assertRaisesRegex(ValueError, "generation output identity"):
                gate.check_candidate(args, root, root, grammar, {}, {})
            (source / "parser.c").write_bytes(b"parser")
            product["identity"]["GrammarRev"] = "other"
            gate.save(module / "provenance.json", product)
            with self.assertRaisesRegex(ValueError, "generation identities differ"):
                gate.check_candidate(args, root, root, grammar, {}, {})

    def test_product_checks_reject_skips_and_missing_tests_and_force_full_product_lane(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "data/corpus").mkdir(parents=True)
            (root / "data/corpus/manifest.csv").write_text("id,lang,source_ref,bytes,sha256\n")
            args = Namespace(lang="swift")
            pins = {"grammars": {"swift": {}, "golang": {}, "json": {}}}
            for label, action in (("ok", "pass"), ("skip", "skip"), ("missing", "output")):
                def execute(command, cwd, log, env, **kwargs):
                    self.assertIn("no_go,no_json", command)
                    self.assertEqual(env["CGO_ENABLED"], "0")
                    self.assertEqual(env["TSG_S15_EDITS_SAMPLE"], "0")
                    self.assertEqual(env["TSG_REQUIRE_FIELD_LOOKUP"], "1")
                    log.write_text("\n".join(json.dumps({"Action": action, "Test": test})
                                             for test in ("TestCorpus", "TestCorpus/swift")) + '\n{"Action": "pass"}\n')
                with patch.object(gate, "execute", execute):
                    if action == "pass":
                        gate.product_conformance(args, root, root, pins, "swift", root / "data",
                                                 ("TestCorpus",), label, {}, {})
                    else:
                        with self.assertRaisesRegex(ValueError, "missing or skipped"):
                            gate.product_conformance(args, root, root, pins, "swift", root / "data",
                                                     ("TestCorpus",), label, {}, {})

    def test_large_product_projection_preserves_input_and_native_dump_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "large/swift").mkdir(parents=True)
            (root / "candidate").mkdir()
            source = root / "large/swift/sample.swift"
            source.write_bytes(b"let value = 1\r\n")
            (root / "candidate/large-sample.swift.dump").write_bytes(b"native dump\n")
            data = gate.large_product_data(root, "swift", [{"lang": "swift", "file": "sample.swift"}])
            row = gate.rows(data / "corpus/manifest.csv")[0]
            self.assertEqual(row["id"], "swift/large-0000")
            self.assertEqual(row["sha256"], gate.sha(source))
            self.assertEqual((data / "corpus/swift/large-0000.in").read_bytes(), source.read_bytes())
            self.assertEqual((data / "native/dumps/swift/large-0000.dump").read_bytes(), b"native dump\n")

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
                with patch.object(gate.proctree, "start", return_value=proc), patch.object(gate, "stop_tree") as stop:
                    with self.assertRaises(type(error)):
                        gate.execute(["child"], tmp, Path(tmp) / "run.log")
                    stop.assert_called_once_with(proc)

    def test_timeout_stops_child_and_grandchild_and_retains_failure_receipt(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "timeout.log"
            pids = Path(tmp) / "pids.json"
            with self.assertRaisesRegex(RuntimeError, "exited 124"):
                gate.execute(tree_command(pids), tmp, log, timeout=1)
            assert_stopped(self, pids)
            receipt = json.loads(log.with_suffix(".command.json").read_text())
            self.assertEqual(receipt["exit"], 124)
            self.assertEqual(receipt["timeoutSeconds"], 1)

    def test_cleanup_failure_still_saves_receipt_and_preserves_interrupt(self):
        with tempfile.TemporaryDirectory() as tmp:
            log = Path(tmp) / "failed.log"
            cases = ((None, RuntimeError, 0), (KeyboardInterrupt(), KeyboardInterrupt, None),
                     (subprocess.TimeoutExpired("child", 1), RuntimeError, 124))
            for failure, expected, code in cases:
                proc = Mock()
                proc.wait.side_effect = failure
                proc.wait.return_value = 0
                with patch.object(gate.proctree, "start", return_value=proc), \
                        patch.object(gate, "stop_tree", side_effect=RuntimeError("cleanup failed")):
                    with self.assertRaises(expected):
                        gate.execute(["child"], tmp, log)
                receipt = json.loads(log.with_suffix(".command.json").read_text())
                self.assertIn("cleanup failed", receipt["cleanupError"])
                self.assertEqual(receipt["exit"], code)
                if isinstance(failure, KeyboardInterrupt):
                    self.assertIn("KeyboardInterrupt", receipt["error"])

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
            self.assertNotEqual(differences[0]["referenceDumpSHA256"], differences[0]["candidateDumpSHA256"])
            original = differences[0]["candidateDumpSHA256"]
            (roots[1] / "edits/lang/case-e1.fields").write_text("changed fields")
            self.assertNotEqual(gate.compare_data(*roots)[0]["candidateDumpSHA256"], original)
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
