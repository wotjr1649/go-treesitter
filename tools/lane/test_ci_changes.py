import json
import os
import re
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
        self.assertEqual(on("windows", f), ["windows_race"])
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
# A job the classifier expects (changes.outputs.expect, a JSON list of job ids) must have succeeded: a skip there is a wiring error.
GATE_CMD = "jq -c 'map_values(.result)' <<<$NEEDS; jq -e --arg ok success --arg sk skipped " \
           "'. as $n | all(.[]; .result == $ok or .result == $sk) and .changes.result == $ok " \
           "and all(.changes.outputs.expect | fromjson[]; $n[.].result == $ok)' <<<$NEEDS"
# The classify step of every changes job: the lane tests first, then the classifier of the base commit (a pull request that edits the
# classifier is judged by the old one and hits its "lib" rule; a base without it runs everything). Same text in each, apart from the workflow.
CHANGES_CMD = """python3 -m unittest discover -s tools/lane -p test_ci_changes.py
cc=tools/lane/ci_changes.py ev=$GITHUB_EVENT_NAME
if [ $ev = pull_request ]; then git show HEAD^1:$cc > $RUNNER_TEMP/base_ci_changes.py 2>/dev/null && cc=$RUNNER_TEMP/base_ci_changes.py || ev=workflow_dispatch; fi
python3 $cc --workflow %s --event $ev"""


def norm(s):  # YAML block scalar or JSON string with \n escapes -> one whitespace-collapsed line
    return " ".join(s.replace("\\n", "\n").split())
GATE_WORKFLOWS = {"engine": "engine-gate", "regen": "regen-gate", "conformance": "conformance-gate", "windows": "windows-gate",
                  "api-guard": "api-guard-gate", "pin-move": "pin-move-gate"}


def workflow_text(wf):
    return (ROOT / ".github" / "workflows" / (wf + ".yml")).read_text(encoding="utf-8")


def job_ids(wf):
    text = workflow_text(wf)
    if wf == "windows":  # JSON in a .yml file
        return list(json.loads(text)["jobs"])
    body = text[text.index("\njobs:\n"):]
    return re.findall(r"^  ([A-Za-z0-9_-]+):\s*$", body, flags=re.M)


def job_conditions(wf):
    """{job id: output keys its job-level "if" requires to be 'true'} for the jobs that have such a condition."""
    text = workflow_text(wf)
    if wf == "windows":
        conds = {j: job.get("if", "") for j, job in json.loads(text)["jobs"].items()}
    else:
        parts = re.split(r"^  ([A-Za-z0-9_-]+):\s*$", text[text.index("\njobs:\n"):], flags=re.M)
        conds = {parts[i]: (re.search(r"^ {4}if: (.*)$", parts[i + 1], flags=re.M) or [None, ""])[1] for i in range(1, len(parts), 2)}
    found = {j: set(re.findall(r"needs\.changes\.outputs\.([A-Za-z_]+) == 'true'", c)) for j, c in conds.items()}
    return {j: keys for j, keys in found.items() if keys}


class WiringTest(unittest.TestCase):
    """A misspelled output name would evaluate to '' and skip a job silently, and the gate would stay green."""

    def test_outputs_used_are_outputs_produced(self):
        for wf in cc.WORKFLOWS:
            produced = {k for k in cc.decide(wf, ["Makefile"], "pull_request") if k != "classes"}
            text = workflow_text(wf)
            used = set(re.findall(r"needs\.changes\.outputs\.([A-Za-z_]+)", text))
            exported = set(re.findall(r"steps\.classify\.outputs\.([A-Za-z_]+)", text))
            self.assertLessEqual(used, produced, wf)
            self.assertLessEqual(exported, produced, wf)
            self.assertIn("expect", exported, wf)  # the gate reads it
            # conformance_full only shapes conformance_matrix (matrix.group.name), no job reads it.
            flags = {k for k in produced if (k in cc.NEEDS and k != "conformance_full") or k in ("engine_shards", "floor_shards", "conformance_matrix")}
            self.assertLessEqual(flags, used, "%s: produced but never used: %s" % (wf, flags - used))

    def test_every_job_waits_for_changes_and_the_gate_waits_for_every_job(self):
        for wf, gate in GATE_WORKFLOWS.items():
            ids = job_ids(wf)
            self.assertIn("changes", ids, wf)
            self.assertIn(gate, ids, wf)
            text = workflow_text(wf)
            if wf == "windows":
                jobs = json.loads(text)["jobs"]
                needs = set(jobs[gate]["needs"])
                for jid, job in jobs.items():
                    if jid not in ("changes", gate):
                        self.assertIn("changes", job["needs"] if isinstance(job["needs"], list) else [job["needs"]], (wf, jid))
            else:
                m = re.search(r"^  %s:\n(?:.*\n)*?    needs: \[([^\]]*)\]" % re.escape(gate), text, flags=re.M)
                self.assertIsNotNone(m, wf)
                needs = {n.strip() for n in m.group(1).split(",")}
            self.assertEqual(needs, set(ids) - {gate}, wf)


    def test_expect_table_matches_the_job_conditions(self):
        for wf in cc.WORKFLOWS:
            self.assertEqual({j: set(k) for j, k in cc.JOB_IF[wf].items()}, job_conditions(wf), wf)
            self.assertLessEqual(set(cc.JOB_IF[wf]), set(job_ids(wf)), wf)

    def test_expect_lists_the_jobs_that_must_run(self):
        self.assertEqual(json.loads(flags("engine", ["README.md"])["expect"]), ["engine-linux-amd64"])
        self.assertEqual(json.loads(flags("regen", ["README.md"])["expect"]), [])
        self.assertEqual(json.loads(flags("engine", ["query.go"])["expect"]),
                         [j for j in cc.JOB_IF["engine"] if "full" not in cc.JOB_IF["engine"][j]])
        self.assertEqual(json.loads(flags("engine", ["README.md"], "push")["expect"]), list(cc.JOB_IF["engine"]))
        self.assertEqual(json.loads(flags("windows", None)["expect"]), ["licenses", "race"])


MATRIX_STEP = "The successful matrix jobs are the expected instances"


def matrix_step(wf):
    """(env mapping, run script) of the matrix-count step of a gate; it is the last step of engine.yml and conformance.yml."""
    text = workflow_text(wf)
    block = text[text.index("- name: %s\n" % MATRIX_STEP):]
    env = dict(re.findall(r"^ {10}([A-Z_]+): (.*)$", block.split("        run: |\n")[0], flags=re.M))
    script = "\n".join(l[10:] for l in block.split("        run: |\n")[1].splitlines())
    return env, script


class MatrixCountTest(unittest.TestCase):
    """The gate counts the successful instances of the dynamic matrices, so a matrix that shrank cannot pass as a full run."""

    def names(self, wf, files, event="pull_request"):
        return json.loads(flags(wf, files, event)["expect_matrix"])

    def test_expected_instances(self):
        self.assertEqual(self.names("conformance", ["README.md"]), ["conformance (ubuntu-24.04, g1)"])
        full = self.names("conformance", ["tools/conformance/a_test.go"])
        self.assertEqual(len(full), 10)
        self.assertIn("conformance (ubuntu-24.04-arm, g5)", full)
        self.assertEqual(self.names("engine", ["README.md"]), [])  # only engine (ubuntu-24.04), which has no matrix
        self.assertEqual(self.names("engine", ["tools/gen/pins.json"]), [])  # no grammar shards: drift is regen's
        pr = self.names("engine", ["query.go"])
        self.assertEqual(len(pr), 5 + 5 + 1)  # linux-amd64 and windows-amd64 shards, one floor shard
        self.assertIn("grammar (ubuntu-24.04, Go 1.26.x, shard 1)", pr)
        self.assertNotIn("grammar (ubuntu-24.04, Go 1.26.x, shard 2)", pr)
        for wf in cc.WORKFLOWS:
            if wf not in ("engine", "conformance"):
                self.assertNotIn("expect_matrix", flags(wf, ["README.md"]))

    def test_full_runs_have_the_full_size(self):
        for wf, files in (("engine", ["README.md"]), ("conformance", ["README.md"])):
            for ev in ("push", "workflow_dispatch", "schedule"):
                env, _ = matrix_step(wf)
                size = int(re.search(r"'(\d+)'", env["FULL"]).group(1))
                self.assertEqual(len(self.names(wf, files, ev)), size, (wf, ev))
        self.assertEqual(len(cc.CONFORMANCE_GROUPS) * len(cc.RUNNERS), 10)
        self.assertEqual(len(cc.ENGINE_MATRIX_JOBS) * len(cc.ENGINE_SHARDS), 30)

    def test_names_match_the_workflows(self):
        text = workflow_text("engine")
        parts = re.split(r"^  ([A-Za-z0-9_-]+):\s*$", text[text.index("\njobs:\n"):], flags=re.M)
        job_text = {parts[i]: parts[i + 1] for i in range(1, len(parts), 2)}
        for job, (tmpl, key) in cc.ENGINE_MATRIX_JOBS.items():
            self.assertIn("    name: %s\n" % (tmpl % 0).replace("0)", "${{ matrix.shard }})"), job_text[job], job)
            # engine_shards sits in a YAML anchor shared by the five platform jobs; the floor job has its own matrix
            self.assertIn("fromJSON(needs.changes.outputs.%s)" % key, job_text[job] if key == "floor_shards" else text, job)
        self.assertIn("    name: conformance (${{ matrix.runner }}, ${{ matrix.group.name }})\n", workflow_text("conformance"))
        self.assertEqual(cc.CONFORMANCE_MATRIX_JOB, "conformance (%s, %s)")

    def test_pattern_selects_only_the_matrix_jobs(self):
        for wf, files, others in (("engine", ["tools/conformance/x.go"], ["engine (ubuntu-24.04)", "engine, oldest supported Go (ubuntu-24.04)", "engine-gate", "engine-changes"]),
                                  ("conformance", ["tools/conformance/x.go"], ["conformance (ubuntu-24.04)", "conformance (ubuntu-24.04-arm)", "conformance-gate", "conformance-changes"])):
            env, _ = matrix_step(wf)
            rx = re.compile(env["MATRIX_RE"].strip("'"))
            self.assertTrue(all(rx.search(n) for n in self.names(wf, files, "push")), wf)
            self.assertFalse(any(rx.search(n) for n in others), wf)

    def test_both_gates_run_the_same_script(self):
        self.assertEqual(matrix_step("engine")[1], matrix_step("conformance")[1])

    def run_step(self, wf, listed, ran, event="pull_request"):
        import shutil
        import tempfile
        bash = shutil.which("bash")
        if not bash or not shutil.which("jq"):
            if sys.platform.startswith("linux"):
                self.fail("bash and jq are needed to run the gate step")
            self.skipTest("bash and jq are needed to run the gate step")
        env, script = matrix_step(wf)
        with tempfile.TemporaryDirectory() as d:
            Path(d, "gh").write_text('#!/bin/sh\ncat "$GH_FIXTURE"\n', newline="\n")
            Path(d, "gh").chmod(0o755)
            Path(d, "jobs.txt").write_text("".join(n + "\n" for n in ran), encoding="utf-8")
            full = re.search(r"'(\d+)'", env["FULL"]).group(1) if event != "pull_request" else ""
            e = dict(os.environ, PATH=d + os.pathsep + os.environ.get("PATH", ""), GH_FIXTURE=str(Path(d, "jobs.txt")),
                     EXPECT=listed, MATRIX_RE=env["MATRIX_RE"].strip("'"), FULL=full, GITHUB_REPOSITORY="o/r", GITHUB_RUN_ID="1")
            res = subprocess.run([bash, "-e", "-c", script], env=e, capture_output=True)
            self.last = (res.stdout + res.stderr).decode("utf-8", "replace").replace("\r", "<CR>")
            return res.returncode

    def test_step_logic(self):
        c = self.names("conformance", ["tools/conformance/x.go"], "push")
        other = ["conformance (ubuntu-24.04)", "conformance-gate", "changes"]  # successful jobs that are not matrix instances
        j = lambda names: json.dumps(names)
        self.assertEqual(self.run_step("conformance", j(c), c + other, "push"), 0, self.last)
        self.assertEqual(self.run_step("conformance", j(c[:1]), c[:1] + other), 0)  # pull request: only g1
        self.assertEqual(self.run_step("conformance", "[]", other), 0)
        self.assertNotEqual(self.run_step("conformance", j(c), c[1:] + other, "push"), 0)  # one instance did not succeed
        self.assertNotEqual(self.run_step("conformance", j(c[:1]), c + other), 0)  # more ran than listed
        # A classifier regression: the matrix and the list shrink together. A push still demands the full size; a pull request cannot tell.
        self.assertNotEqual(self.run_step("conformance", j(c[:1]), c[:1] + other, "push"), 0)
        self.assertNotEqual(self.run_step("conformance", j(c[:9]), c[:9] + other, "push"), 0)
        for bad in ("", "null", "{}", "conformance"):
            self.assertNotEqual(self.run_step("conformance", bad, c + other, "push"), 0, bad)
        e = self.names("engine", ["README.md"], "push")
        self.assertEqual(len(e), 30)
        self.assertEqual(self.run_step("engine", j(e), e + ["engine (ubuntu-24.04)"], "push"), 0)
        self.assertNotEqual(self.run_step("engine", j(e[:25]), e[:25], "push"), 0)  # floor shards dropped on a push
        self.assertNotEqual(self.run_step("engine", j(e), e[:-1], "push"), 0)


class GateTest(unittest.TestCase):
    def test_every_changes_job_runs_the_lane_tests_and_the_base_classifier(self):
        for wf in GATE_WORKFLOWS:
            self.assertIn(norm(CHANGES_CMD % wf), norm(workflow_text(wf)), wf)

    def test_every_required_workflow_has_the_gate_and_a_changes_job(self):
        for wf, gate in GATE_WORKFLOWS.items():
            text = (ROOT / ".github" / "workflows" / (wf + ".yml")).read_text(encoding="utf-8")
            self.assertIn(GATE_CMD, text, wf)
            self.assertIn("name: %s\n" % gate if wf != "windows" else '"name": "%s"' % gate, text, wf)
            self.assertIn("--workflow %s" % wf, text, wf)
            self.assertEqual(text.count(GATE_CMD), 1, wf)

    def test_main_pushes_share_one_group_and_never_cancel_a_running_run(self):
        group = "github.event_name == 'push' && github.run_attempt == 1 && 'main'"  # one pending run per workflow; re-runs and dispatches keep their own group
        for wf in GATE_WORKFLOWS:
            text = norm(workflow_text(wf))
            self.assertIn(group, text, wf)
            cancel = re.search(r'cancel-in-progress"?: "?\$\{\{ (.*?) \}\}', text).group(1)
            self.assertTrue(cancel.startswith("github.event_name == 'pull_request' &&"), wf)

    def run_cmd(self, cmd, needs, expect="[]"):
        import shutil
        bash, jq = shutil.which("bash"), shutil.which("jq")
        if not bash or not jq:
            if sys.platform.startswith("linux"):  # the gate runs on ubuntu: a runner without them must not pass silently
                self.fail("bash and jq are needed to run the gate step")
            self.skipTest("bash and jq are needed to run the gate step")
        outputs = {} if expect is None else {"expect": expect}
        env = dict(os.environ, NEEDS=json.dumps({k: {"result": v, "outputs": outputs if k == "changes" else {}} for k, v in needs.items()}))
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

    def test_gate_expect(self):
        ok = {"changes": "success", "a": "success", "b": "skipped"}
        cases = [
            (ok, '["a"]', 0),
            (ok, '["a","b"]', 1),  # b was expected to run but was skipped: a misspelled condition
            (dict(ok, a="skipped"), '["a"]', 1),
            (dict(ok, a="failure"), '["a"]', 1),
            (ok, '["a","gone"]', 1),  # an expected job that is not in needs
            (ok, "[]", 0),
            (ok, "", 1),  # classifier output missing or empty
            (ok, None, 1),
        ]
        for needs, expect, want in cases:
            self.assertEqual(self.run_cmd(GATE_CMD, needs, expect) != 0, bool(want), (needs, expect))  # jq exits 5 on bad JSON


if __name__ == "__main__":
    unittest.main()
