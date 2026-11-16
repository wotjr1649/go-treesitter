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

    def test_postgres_addition_checks_new_schema_without_old_native(self):
        pins = json.loads(Path(__file__).with_name("pins.json").read_bytes())
        old = {"grammars": {k: v for k, v in pins["grammars"].items() if k != "postgres"}}
        self.assertTrue(pins["grammars"]["postgres"]["regenerate"])
        self.assertFalse(pins["grammars"]["postgres"].get("patches"))
        self.assertEqual(pin_move.changed_grammars(old, pins), [("postgres", "ADDED")])
        with tempfile.TemporaryDirectory() as temp:
            review = pin_move.Review(Path(temp), Path(temp) / "cache")
            with patch.object(review, "side") as side:
                result = review.grammar("base", "head", old, pins, "postgres", "ADDED", set())
            side.assert_called_once_with("head", pins, "postgres", "new", build=False)
            self.assertEqual(result["schema"], "PASS (new only)")
            self.assertTrue(result["pass"])

    def test_non_grammar_metadata_is_not_a_pin_move(self):
        self.assertEqual(pin_move.changed_grammars(
            {"grammars": {}, "mirrors": ["old"]},
            {"grammars": {}, "mirrors": ["new"]},
        ), [])


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
        return subprocess.run(["git", "-c", "commit.gpgsign=false", *args], cwd=repo, env=env, check=True,
                              capture_output=True, text=True).stdout.strip()

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


if __name__ == "__main__":
    unittest.main()
