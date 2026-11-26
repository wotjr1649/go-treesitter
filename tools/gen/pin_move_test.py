"""Focused regression tests for pin selection and the pin-move failure policy."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "pin_move", Path(__file__).with_name("pin-move.py")
)
pin_move = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pin_move)


def dump(has_error=0, node_type="source_file"):
    return (
        f"#tsg-dump/1 root_has_error={has_error} nodes=1\n"
        f"0\t-\t{node_type}\t1\t0\t0\t0\t{has_error}\t0\t1\t0:0\t0:1\n"
    ).encode()


class PinsDiffTest(unittest.TestCase):
    def test_selects_added_removed_and_every_changed_entry_field(self):
        old = {"grammars": {
            "unchanged": {"commit": "a", "files": {"parser.c": "1"}},
            "removed": {"commit": "b"},
            "commit": {"commit": "c"},
            "source": {"repo": "old", "commit": "d"},
            "patch": {"commit": "e", "patches": []},
            "regenerate": {"commit": "g", "regenerate": False},
        }}
        new = {"grammars": {
            "patch": {"commit": "e", "patches": [{"file": "fix.patch"}]},
            "source": {"repo": "new", "commit": "d"},
            "commit": {"commit": "new"},
            "added": {"commit": "f"},
            "regenerate": {"commit": "g", "regenerate": True},
            "unchanged": {"files": {"parser.c": "1"}, "commit": "a"},
        }}
        self.assertEqual(pin_move.changed_grammars(old, new), [
            ("added", "ADDED"), ("commit", "CHANGED"),
            ("patch", "CHANGED"), ("regenerate", "CHANGED"), ("removed", "REMOVED"),
            ("source", "CHANGED"),
        ])

    def test_unpatched_regeneration_addition_checks_new_schema_without_old_native(self):
        pins = json.loads(Path(__file__).with_name("pins.json").read_bytes())
        for lang in ("json", "swift"):  # unpatched regenerated grammars (postgres now carries patches)
            with self.subTest(lang=lang):
                old = {"grammars": {k: v for k, v in pins["grammars"].items() if k != lang}}
                self.assertTrue(pins["grammars"][lang]["regenerate"])
                self.assertFalse(pins["grammars"][lang].get("patches"))
                self.assertEqual(pin_move.changed_grammars(old, pins), [(lang, "ADDED")])
                with tempfile.TemporaryDirectory() as temp:
                    review = pin_move.Review(Path(temp), Path(temp) / "cache")
                    with patch.object(review, "side") as side:
                        result = review.grammar("base", "head", old, pins, lang, "ADDED", set())
                    side.assert_called_once_with("head", pins, lang, "new", build=False)
                    self.assertEqual(result["schema"], "PASS (new only)")
                    self.assertTrue(result["pass"])

    def test_non_grammar_metadata_is_not_a_pin_move(self):
        self.assertEqual(pin_move.changed_grammars(
            {"grammars": {}, "mirrors": ["old"]},
            {"grammars": {}, "mirrors": ["new"]},
        ), [])


class ToolchainTest(unittest.TestCase):
    PINS = json.loads(Path(__file__).with_name("pins.json").read_bytes())

    def moved(self, edit):
        new = json.loads(json.dumps(self.PINS))
        edit(new)
        return pin_move.toolchain_changes(self.PINS, new)

    def test_every_non_grammar_pin_is_classified(self):
        cases = [
            (lambda p: p["runtime"].update(commit="x"), ["runtime"]),
            (lambda p: p["tools"].update(treeSitterCLI="9"), ["tree-sitter-cli"]),
            (lambda p: p["tools"]["treeSitterCLIArchives"]["linux-amd64"].update(sha256="x"), ["tree-sitter-cli"]),
            (lambda p: p["tools"].update(clang="9"), ["wasi-sdk"]),
            (lambda p: p["tools"]["binaryenArchives"]["linux-amd64"].update(sha256="x"), ["binaryen"]),
            (lambda p: p["tools"].update(wasm2go="v9", wasm2goSum="h1:x"), ["wasm2go"]),
            (lambda p: p["wasiLibc"].update(commit="x"), ["wasi-libc"]),
            (lambda p: p.update(shimSHA256="x"), ["shim"]),
            (lambda p: p.update(memoryGrowSHA256="x"), ["memory-grow"]),
            (lambda p: p.update(abi=5, recipe="v3"), ["abi", "recipe"]),
            (lambda p: p["tools"].update(newTool="1"), ["newTool"]),  # unknown pins fail closed
            (lambda p: p.update(newTop=1), ["newTop"]),
            (lambda p: p["tools"].pop("wasm2go"), ["wasm2go"]),
        ]
        for edit, want in cases:
            with self.subTest(want=want):
                self.assertEqual(self.moved(edit), want)

    def test_grammar_mirror_and_host_hash_changes_are_not_toolchain_moves(self):
        for edit in (lambda p: p["grammars"]["json"].update(commit="x"), lambda p: p.update(mirrors=["m"]),
                     lambda p: p["tools"].update(localBinarySHA256={}), lambda p: None):
            self.assertEqual(self.moved(edit), [])

    def test_ack_lines_use_the_override_parser(self):
        body = "x\nPIN-MOVE-TOOLCHAIN-OK wasm2go\n```\nPIN-MOVE-TOOLCHAIN-OK runtime\n```\nPIN-MOVE-ALLOW-OK-TO-ERROR c/new/x.in\n"
        self.assertEqual(pin_move.overrides(body, pin_move.TOOLCHAIN_ACK), {"wasm2go"})
        self.assertEqual(pin_move.overrides(body), {"c/new/x.in"})

    def test_current_pins_agree_with_every_committed_provenance(self):
        # A half-done move (pins edited, modules not regenerated) fails here; skip where the checkout lacks git history.
        try:
            gaps = pin_move.provenance_gaps(pin_move.ROOT, "HEAD", self.PINS, {c for c, _, _ in pin_move.PROVENANCE})
        except (OSError, ValueError, subprocess.SubprocessError):
            self.skipTest("no git checkout")
        self.assertEqual(gaps, [])

    def review_repo(self, temp, tool="v1"):
        repo = Path(temp)
        MergeBaseTest().git(repo, "init", "-q")
        (repo / "grammars/c").mkdir(parents=True)
        (repo / "grammars/c/provenance.json").write_text(json.dumps(
            {"abi": 4, "identity": {"Runtime": "r", "SourceSHA256": {}, "Tools": {"wasm2go": tool, "wasm2go-sum": "s"}}}), encoding="utf-8")
        MergeBaseTest().git(repo, "add", "-A")
        MergeBaseTest().git(repo, "commit", "-qm", "m")
        return repo

    def test_review_fails_closed_until_acknowledged_and_regenerated(self):
        old = {"grammars": {"c": {}}, "tools": {"wasm2go": "v0", "wasm2goSum": "s"}}
        new = {"grammars": {"c": {}}, "tools": {"wasm2go": "v1", "wasm2goSum": "s"}}
        with tempfile.TemporaryDirectory() as temp:
            repo = self.review_repo(temp)
            ok = pin_move.toolchain_review(repo, "HEAD", old, new, {"wasm2go"})
            self.assertEqual((ok["components"], ok["pass"]), (["wasm2go"], True))
            silent = pin_move.toolchain_review(repo, "HEAD", old, new, set())
            self.assertEqual((silent["missing_ack"], silent["pass"]), (["wasm2go"], False))
            stale = pin_move.toolchain_review(repo, "HEAD", old, {**new, "tools": {**new["tools"], "wasm2go": "v2"}}, {"wasm2go", "runtime"})
            self.assertEqual((stale["pass"], stale["unused_ack"]), (False, ["runtime"]))
            self.assertEqual(stale["provenance_gaps"], ["c: identity.Tools.wasm2go is not the pinned v2"])
            self.assertEqual(pin_move.toolchain_review(repo, "HEAD", old, old, set())["pass"], True)
            missing = pin_move.toolchain_review(repo, "HEAD", old, {**new, "grammars": {"c": {}, "d": {}}}, {"wasm2go"})
            self.assertEqual(missing["provenance_gaps"], ["d: provenance.json missing"])

    def test_summary_names_the_evidence(self):
        report = {"status": "FAIL", "grammars": [], "toolchain": {"components": ["wasm2go"], "missing_ack": ["wasm2go"],
                                                                  "provenance_gaps": [], "unused_ack": [], "pass": False}}
        text = pin_move.summarize(report)
        self.assertIn("Toolchain move: wasm2go", text)
        self.assertIn("regen drift", text)
        self.assertIn("PIN-MOVE-TOOLCHAIN-OK wasm2go", text)


class ClassificationTest(unittest.TestCase):
    def test_raw_token_names_do_not_split_native_records(self):
        for token in ("\n", "\t"):
            with self.subTest(token=repr(token)):
                tree = dump().replace(b"nodes=1", b"nodes=2") + (
                    f"1\t-\t{token}\t0\t0\t0\t0\t0\t0\t1\t0:0\t0:1\n"
                ).encode()
                self.assertEqual(pin_move.classify(tree, tree), "IDENTICAL")
                malformed = [
                    tree.replace(b"nodes=2", b"nodes=1"),
                    tree.replace(b"nodes=2", b"nodes=3"),
                    tree.replace(b"\n1\t-", b"\n2\t-"),
                    tree[:-1],
                ]
                for bad in malformed:
                    with self.subTest(malformed=bad), self.assertRaises(ValueError):
                        pin_move.classify(bad, bad)

    def test_tree_changes_and_error_transitions(self):
        cases = [
            (dump(), dump(), "IDENTICAL"),
            (dump(), dump(node_type="other"), "CHANGED"),
            (dump(1), dump(1, "other"), "CHANGED"),
            (dump(), dump(1), "OK_TO_ERROR"),
            (dump(1), dump(), "ERROR_TO_OK"),
        ]
        for old, new, expected in cases:
            with self.subTest(expected=expected):
                self.assertEqual(pin_move.classify(old, new), expected)

    def test_malformed_dump_fails_on_either_side_even_when_identical(self):
        malformed = [
            b"",
            b"#tsg-dump/2 root_has_error=0 nodes=1\n",
            dump().replace(b"root_has_error=0", b"root_has_error=2"),
            dump().replace(b"nodes=1", b"nodes=2"),
            dump(1).replace(b"root_has_error=1", b"root_has_error=0"),
            dump().split(b"\n", 1)[0] + b"\n",
            dump().replace(b"\t0:0\t0:1", b"\t0:0"),
        ]
        for bad in malformed:
            for old, new in [(bad, dump()), (dump(), bad), (bad, bad)]:
                with self.subTest(old=old, new=new):
                    with self.assertRaises(ValueError):
                        pin_move.classify(old, new)


class OverrideTest(unittest.TestCase):
    def test_html_comment_cannot_hide_an_override(self):
        self.assertEqual(pin_move.overrides("\n".join([
            "<!--",
            "PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            "-->",
            "PIN-MOVE-ALLOW-OK-TO-ERROR dart/old/sample-001.in",
        ])), {"dart/old/sample-001.in"})

    def test_only_explicit_visible_whole_lines_override(self):
        self.assertEqual(pin_move.overrides("\n".join([
            "PIN-MOVE-ALLOW-OK-TO-ERROR php/new/expressions-027.in",
            "PIN-MOVE-ALLOW-OK-TO-ERROR php/new/expressions-027.in",
            "- PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            "Prose PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            " PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            "PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c explanation",
            "```text",
            "PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            "```",
            "~~~~text",
            "PIN-MOVE-ALLOW-OK-TO-ERROR c/large/openssl.c",
            "~~~~",
            "PIN-MOVE-ALLOW-OK-TO-ERROR dart/old/sample-001.in",
        ])), {"php/new/expressions-027.in", "dart/old/sample-001.in"})


class OverrideLineBreakTest(unittest.TestCase):
    LINE = "PIN-MOVE-ALLOW-OK-TO-ERROR c/new/x.in"

    def test_only_lf_ends_a_line(self):
        for char in (" ", " ", "\x85", "\x0b", "\x0c", "\x1c", "\x1d", "\x1e"):
            with self.subTest(char=repr(char)):
                self.assertEqual(pin_move.overrides("see the table" + char + self.LINE), set())
                self.assertEqual(pin_move.overrides(self.LINE + char + "tail"), set())

    def test_crlf_and_lf_lines_still_count(self):
        self.assertEqual(pin_move.overrides("intro\r\n" + self.LINE + "\r\nmore\n"), {"c/new/x.in"})


class MergeBaseTest(unittest.TestCase):
    def git(self, repo, *args):
        env = dict(os.environ, GIT_AUTHOR_NAME="t", GIT_AUTHOR_EMAIL="t@example.invalid",
                   GIT_COMMITTER_NAME="t", GIT_COMMITTER_EMAIL="t@example.invalid")
        return pin_move.proctree.run(["git", "-c", "commit.gpgsign=false", *args], cwd=repo, env=env, check=True,
                              capture_output=True, text=True, timeout=120).stdout.strip()

    def commit(self, repo, commit, grammar_commit=None, orphan=False):
        if orphan:
            self.git(repo, "checkout", "--orphan", "other")
        pins = repo / "tools/gen/pins.json"
        pins.parent.mkdir(parents=True, exist_ok=True)
        pins.write_text(json.dumps({"grammars": {"c": {"commit": grammar_commit or "a"}}}), encoding="utf-8")
        (repo / "f.txt").write_text(commit, encoding="utf-8")
        self.git(repo, "add", "-A")
        self.git(repo, "commit", "-m", commit)
        return self.git(repo, "rev-parse", "HEAD")

    def test_stale_branch_is_not_judged_against_a_later_pin_move(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            self.git(repo, "init", "-q")
            fork = self.commit(repo, "fork point")
            self.git(repo, "checkout", "-q", "-b", "pr")
            head = self.commit(repo, "pr change, pins untouched")  # same pins as the fork point
            self.git(repo, "checkout", "-q", "-")
            base = self.commit(repo, "base moved a pin afterwards", grammar_commit="b")
            # The tip comparison sees a reverse move; the merge base sees none.
            tip = pin_move.commits(repo, base, head)
            self.assertEqual(tip, (base, head))
            self.assertEqual(pin_move.changed_grammars(*(pin_move.pins_at(repo, c) for c in tip)), [("c", "CHANGED")])
            merged = pin_move.commits(repo, base, head, merge_base=True)
            self.assertEqual(merged, (fork, head))
            self.assertEqual(pin_move.changed_grammars(*(pin_move.pins_at(repo, c) for c in merged)), [])

    def test_missing_merge_base_fails_closed(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            self.git(repo, "init", "-q")
            base = self.commit(repo, "one")
            head = self.commit(repo, "unrelated", orphan=True)
            self.assertEqual(pin_move.commits(repo, base, head), (base, head))
            with self.assertRaisesRegex(ValueError, "merge-base"):
                pin_move.commits(repo, base, head, merge_base=True)

    def test_unknown_ref_fails(self):
        with tempfile.TemporaryDirectory() as temp:
            repo = Path(temp)
            self.git(repo, "init", "-q")
            head = self.commit(repo, "one")
            with self.assertRaises(ValueError):
                pin_move.commits(repo, "no-such-ref", head, merge_base=True)


class GrammarPolicyTest(unittest.TestCase):
    def test_missing_large_input_baseline_fails_before_fetch(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            review = pin_move.Review(root / "out", root / "cache")
            with patch.object(review, "command") as command:
                with self.assertRaisesRegex(ValueError, "missing large-input baseline"):
                    review.large_inputs("unregistered-grammar", root / "data", root / "large")
                command.assert_not_called()

    def prepare_review(self, root, missing_old=False):
        review = pin_move.Review(root / "out", root / "cache")
        review.tools = {"gen": "unused-gen"}
        folders = {side: root / side for side in ("old", "new")}
        for side, folder in folders.items():
            corpus = folder / "data/corpus"
            corpus.mkdir(parents=True)
            (corpus / "c").mkdir()
            source = b"x"
            (corpus / "c/sample.in").write_bytes(source)
            rows = "" if missing_old and side == "old" else f"c/sample,{pin_move.sha(source)}\n"
            (corpus / "manifest.csv").write_text("id,sha256\n" + rows, encoding="utf-8")
            (corpus / "skipped.csv").write_text("id,reason\n", encoding="utf-8")
        return review, folders

    def test_override_only_allows_a_matching_ok_to_error(self):
        exact = {"c/old/sample.in", "c/new/sample.in"}
        cases = [
            ("no override", set(), dump(1), 0, False, "OK_TO_ERROR"),
            ("exact override", exact, dump(1), 0, True, "OK_TO_ERROR"),
            ("unrelated override", {"c/new/other.in"}, dump(1), 0, False, "OK_TO_ERROR"),
            ("malformed native", exact, b"not a dump\n", 0, False, "UNPARSABLE"),
            ("nonzero native", exact, dump(1), 7, False, "UNPARSABLE"),
        ]
        for label, allowed, new_dump, new_exit, passed, classification in cases:
            with self.subTest(case=label), tempfile.TemporaryDirectory() as temp:
                review, folders = self.prepare_review(Path(temp))

                def command(args, **kwargs):
                    if args[1] == "schema":
                        return b'{"differences": []}', 0
                    return (dump(), 0) if Path(args[0]).parent.parent.name == "old" else (new_dump, new_exit)

                with patch.object(review, "side", side_effect=lambda ref, pins, lang, side, **kw: folders[side]), \
                        patch.object(review, "command", side_effect=command), \
                        patch.object(review, "large_inputs", return_value=[]):
                    row = review.grammar("base", "head", {}, {}, "c", "CHANGED", allowed)
                self.assertEqual(row["pass"], passed)
                self.assertEqual(row["counts"], {classification: 2})
                self.assertEqual([r["override"] for r in row["results"]], [passed, passed])

    def test_missing_old_corpus_baseline_fails(self):
        with tempfile.TemporaryDirectory() as temp:
            review, folders = self.prepare_review(Path(temp), missing_old=True)
            with patch.object(review, "side", side_effect=lambda ref, pins, lang, side, **kw: folders[side]), \
                    patch.object(review, "command", return_value=(b'{"differences": []}', 0)):
                with self.assertRaisesRegex(ValueError, "missing corpus baseline"):
                    review.grammar("base", "head", {}, {}, "c", "CHANGED", {"c/new/sample.in"})


class CommandTimeoutTest(unittest.TestCase):
    def test_timeout_reaps_the_process_and_records_failure(self):
        processes = []
        popen = pin_move.subprocess.Popen

        def track_process(*args, **kwargs):
            process = popen(*args, **kwargs)
            processes.append(process)
            return process

        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            review = pin_move.Review(root / "out", root / "cache")
            with patch.object(pin_move.subprocess, "Popen", side_effect=track_process):
                _, code = review.command(
                    [sys.executable, "-c", "import time; time.sleep(30)"],
                    timeout=0.05, check=False,
                )
            self.assertEqual(code, "TIMEOUT")
            self.assertTrue(processes)
            self.assertTrue(all(process.poll() is not None for process in processes))
            receipt = json.loads((review.out / "logs/0001.json").read_text(encoding="utf-8"))
            self.assertEqual(receipt["exit"], "TIMEOUT")



def archive(root, name, members):
    """A .tar.gz in root; returns its sha256 (members: {path: bytes})."""
    import io, tarfile
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        for member, data in members.items():
            info = tarfile.TarInfo(member)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    (root / name).write_bytes(buffer.getvalue())
    return pin_move.sha(buffer.getvalue())


class PhaseTest(unittest.TestCase):
    """S140: --phase fetch downloads into the cache and runs nothing; --phase run is offline and sandboxed."""

    def pins(self, root):
        grammar = archive(root, "g.tgz", {"top/src/node-types.json": b"[]"})
        runtime, wasi = archive(root, "rt.tgz", {"a": b"1"}), archive(root, "wasi.tgz", {"a": b"2"})
        return {"runtime": {"archive": "rt.tgz", "archiveSHA256": runtime},
                "wasiLibc": {"archive": "wasi.tgz", "archiveSHA256": wasi},
                "grammars": {"c": {"archive": "g.tgz", "archiveSHA256": grammar, "grammarDir": "src"}}}

    def cache_all(self, root, pins):
        for source in (pins["runtime"], pins["wasiLibc"], pins["grammars"]["c"]):
            pin_move.write(root / "cache/archives" / source["archiveSHA256"], (root / source["archive"]).read_bytes())

    def test_constructor_pairs_run_with_a_sandbox(self):
        for phase, sandbox in [("run", None), (None, Path("s.sh")), ("fetch", Path("s.sh")), ("other", None)]:
            with self.subTest(phase=phase, sandbox=sandbox), self.assertRaises(ValueError):
                pin_move.Review(Path("out"), Path("cache"), phase, sandbox)
        pin_move.Review(Path("out"), Path("cache"))
        pin_move.Review(Path("out"), Path("cache"), "fetch")
        pin_move.Review(Path("out"), Path("cache"), "run", Path("s.sh"))

    def test_cli_rejects_run_without_sandbox_and_sandbox_without_run(self):
        for flags in (["--phase", "run"], ["--sandbox", "s.sh"], ["--phase", "fetch", "--sandbox", "s.sh"]):
            with self.subTest(flags=flags), patch.object(sys, "argv", ["pin-move.py", *flags, "--out", "o", "--cache", "c"]), \
                    patch.object(sys, "stderr"), self.assertRaises(SystemExit) as stop:
                pin_move.main()
            self.assertEqual(stop.exception.code, 2)

    def test_sandbox_wraps_only_commands_with_mounts_and_keeps_the_logical_receipt(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            review = pin_move.Review(root / "out", root / "cache", "run", root / "sandbox-run.sh")
            seen = []

            def run(args, **kwargs):
                seen.append(list(args))
                return subprocess.CompletedProcess(args, 0)

            with patch.object(pin_move.proctree, "run", side_effect=run):
                review.command(["tool", "a"], cwd=root, mounts=["--ro", root / "in"])
                review.command(["git", "status"], cwd=root)
            self.assertEqual(seen[0], ["bash", root / "sandbox-run.sh", "--ro", root / "in", "--chdir", root, "--", "tool", "a"])
            self.assertEqual(seen[1], ["git", "status"])
            wrapped = json.loads((review.out / "logs/0001.json").read_text(encoding="utf-8"))
            self.assertEqual(wrapped["args"], ["tool", "a"])
            self.assertEqual(wrapped["sandbox"][:2], ["bash", str(root / "sandbox-run.sh")])
            self.assertNotIn("sandbox", json.loads((review.out / "logs/0002.json").read_text(encoding="utf-8")))

    def test_side_fetch_stages_into_the_cache_and_builds_nothing(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            pins = self.pins(root)
            review = pin_move.Review(root / "out", root / "cache", "fetch")
            review.tools = {"native-ref": "native-ref"}
            calls = []

            def command(args, **kwargs):  # -stage downloads: the archives and one npm tarball appear in dl
                calls.append([str(x) for x in args])
                dl = Path(args[args.index("-dl") + 1])
                for source in (pins["runtime"], pins["wasiLibc"], pins["grammars"]["c"]):
                    pin_move.write(dl / source["archive"], (root / source["archive"]).read_bytes())
                pin_move.write(dl / "npm" / "x.tgz", b"npm")
                pin_move.write(Path(args[args.index("-work") + 1]) / "extracted", b"x")
                return b"", 0

            with patch.object(review, "command", side_effect=command):
                folder = review.side("head", pins, "c", "new", build=False)
            self.assertEqual(len(calls), 1)
            self.assertIn("-stage", calls[0])
            for flag in ("-fetch", "-nobuild", "-out", "-corpus"):
                self.assertNotIn(flag, calls[0])
            self.assertFalse((folder / "stage-work").exists())
            for source in (pins["runtime"], pins["wasiLibc"], pins["grammars"]["c"]):  # wasi-libc too, although build=False
                self.assertTrue((root / "cache/archives" / source["archiveSHA256"]).exists())
            self.assertEqual((root / "cache/npm/x.tgz").read_bytes(), b"npm")

    def test_side_run_is_offline_and_mounts_only_the_build_directories_writable(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            pins = self.pins(root)
            self.cache_all(root, pins)
            pin_move.write(root / "cache/npm/x.tgz", b"npm")
            review = pin_move.Review(root / "out", root / "cache", "run", root / "sandbox-run.sh")
            review.tools = {"native-ref": root / "native-ref", "gen": "gen", "ciconf": "ciconf"}
            calls = []

            def command(args, **kwargs):
                calls.append((list(args), kwargs.get("mounts")))
                return b"{}", 0

            with patch.object(review, "command", side_effect=command):
                folder = review.side("head", pins, "c", "new")
            build = calls[0]
            self.assertNotIn("-fetch", build[0])
            self.assertEqual(build[1][:6], ["--ro", pin_move.ROOT, "--ro", root / "native-ref", "--ro", folder])
            self.assertEqual(build[1][6:], ["--rw", folder / "work", "--rw", folder / "exe", "--rw", folder / "corpus"])
            self.assertTrue(all(mounts is None for _, mounts in calls[1:]))  # first-party schema check and ciconf stay on the host
            self.assertEqual((folder / "dl/npm/x.tgz").read_bytes(), b"npm")

    def test_large_inputs_download_only_in_fetch_and_never_in_run(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            fetched = []

            def command(args, **kwargs):  # benchci fetch: write each manifest entry's bytes under -out
                subset = json.loads(Path(args[args.index("-manifest") + 1]).read_text(encoding="utf-8"))
                fetched.append([e["kind"] for e in subset])
                return b"", 0

            fetch = pin_move.Review(root / "out", root / "cache", "fetch")
            fetch.tools = {"benchci": "benchci"}
            with patch.object(fetch, "command", side_effect=command):
                self.assertEqual(fetch.large_inputs("json", "", root / "f"), [])  # json: derived, nothing to download
                with self.assertRaises(FileNotFoundError):  # golang: url entries are requested, the stub wrote none
                    fetch.large_inputs("golang", "", root / "f")
            self.assertEqual(len(fetched), 1)
            self.assertTrue(set(fetched[0]) <= set(pin_move.NETWORK_KINDS))

            run = pin_move.Review(root / "out2", root / "cache", "run", root / "sandbox-run.sh")
            run.tools = {"benchci": "benchci"}
            with patch.object(run, "command") as command:
                with self.assertRaisesRegex(ValueError, "not in the cache"):
                    run.large_inputs("golang", root / "data", root / "r")
                command.assert_not_called()

    def test_grammar_fetch_stages_both_sides_and_compares_nothing(self):
        for status, sides, large in (("CHANGED", 2, 1), ("ADDED", 1, 0), ("REMOVED", 0, 0)):
            with self.subTest(status=status), tempfile.TemporaryDirectory() as temp:
                review = pin_move.Review(Path(temp) / "out", Path(temp) / "cache", "fetch")
                with patch.object(review, "side") as side, patch.object(review, "large_inputs") as large_inputs, \
                        patch.object(review, "command") as command:
                    row = review.grammar("base", "head", {}, {}, "c", status, set())
                self.assertTrue(row["pass"])
                self.assertEqual((side.call_count, large_inputs.call_count), (sides, large))
                command.assert_not_called()

    def test_native_dump_is_sandboxed_with_only_the_grammar_tree_mounted(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            review = pin_move.Review(root / "out", root / "cache", "run", root / "sandbox-run.sh")
            review.tools = {"gen": "gen"}
            folders = {}
            for side in ("old", "new"):
                folders[side] = review.out / "c" / side
                corpus = folders[side] / "data/corpus"
                (corpus / "c").mkdir(parents=True)
                (corpus / "c/sample.in").write_bytes(b"x")
                (corpus / "manifest.csv").write_text(f"id,sha256\nc/sample,{pin_move.sha(b'x')}\n", encoding="utf-8")
                (corpus / "skipped.csv").write_text("id,reason\n", encoding="utf-8")
            dumps = []

            def command(args, **kwargs):
                if args[1] == "schema":
                    return b'{"differences": []}', 0
                dumps.append((kwargs.get("mounts"), kwargs.get("cwd")))
                return dump(), 0

            with patch.object(review, "side", side_effect=lambda ref, pins, lang, side, **kw: folders[side]), \
                    patch.object(review, "command", side_effect=command), patch.object(review, "large_inputs", return_value=[]):
                row = review.grammar("base", "head", {}, {}, "c", "CHANGED", set())
            self.assertTrue(row["pass"])
            self.assertEqual(dumps, [(["--ro", review.out / "c"], review.out / "c")] * 4)  # old and new exe over both sides' inputs


if __name__ == "__main__":
    unittest.main()
