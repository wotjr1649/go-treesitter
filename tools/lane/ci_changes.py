#!/usr/bin/env python3
"""Classify a pull request's changed files and decide which required CI jobs must run (CI load reduction).

Every PR-required workflow starts with a cheap "changes" job that runs this script and feeds its outputs to job-level
"if" conditions (a job skipped by "if" counts as passing for a required check; a workflow-level paths filter would leave
the check pending forever) and, for engine.yml, regen.yml and conformance.yml, to dynamic matrices. A gate job per workflow then
fails when "changes" or any needed job failed or was cancelled, or when a job the "expect" output lists did not succeed. The engine, regen and
conformance gates also count the matrix instances that succeeded against "expect_matrix" (and, outside pull requests, against the full size).

    ci_changes.py --workflow engine|regen|conformance|windows|api-guard|pin-move [--files FILE]

A pull request that does not edit this script is classified by it (it is the base commit's copy, since HEAD^1 and HEAD hold the same
bytes), so a pull request cannot change its own classification by any other file. A pull request that edits it (git diff --quiet
HEAD^1 HEAD -- tools/lane/ci_changes.py is false, or HEAD^1 cannot be read) runs this script of the pull request in run-everything mode
(--event workflow_dispatch): every job, the whole matrices, the full gate sizes, described by the pull request's own tables and workflows,
whose agreement the lane tests (run first in every changes job) check. The base copy cannot be used there: its expect lists and matrix
outputs name the base's jobs, which the pull request's workflows may not have.

Changed files come from "git diff --name-only --no-renames HEAD^1 HEAD" on the pull request's merge commit (the default
checkout of a pull_request event; fetch-depth 2), or from --files ("-" is stdin; tests). Anything unknown, an empty or
unreadable diff, a change to the workflow itself or to this script, and every event except pull_request run everything.
Docs are not skipped blindly: tools/conformance TestKnownGap* reads every tracked file, and the root TestAPIInventory
reads docs/design/api-stability.md, so conformance always keeps one job (g1, amd64) on a pull request.

Outputs go to $GITHUB_OUTPUT (or stdout) as key=value lines. Matrices are JSON for fromJSON().
"""
import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path

WORKFLOWS = ("engine", "regen", "conformance", "windows", "api-guard", "pin-move")

# Grammar partitions. They must name every grammars/* directory exactly once (checked here and in the unit test).
# Two shards balanced by the measured per-grammar test time of main push runs (powershell and swift take most of it; the other 23 grammars
# are a few seconds each), so each job pays checkout and Go setup once instead of five times.
ENGINE_SHARDS = [
    {"shard": 1, "grammars": "powershell cpp css postgres php golang dart typescript tsx rust r json"},
    {"shard": 2, "grammars": "swift bash kotlin tsql csharp ruby c xml javascript html java python yaml"},
]
# Split by measured run time; g1 also runs the repository-wide checks once per architecture.
CONFORMANCE_GROUPS = [
    {"name": "g1", "docs": True, "langs": "tsql css c"},
    {"name": "g2", "langs": "postgres ruby golang php"},
    {"name": "g3", "langs": "swift yaml dart tsx json r"},
    {"name": "g4", "langs": "powershell typescript java javascript rust xml"},
    {"name": "g5", "langs": "kotlin csharp cpp bash python html"},
]
# regen.yml groups, balanced by the measured regenerate-and-compare time per grammar (postgres alone: about 11 of its 12 minutes);
# each group pays checkout, Go setup and the toolchain build once.
REGEN_GROUPS = [
    {"name": "g1", "langs": "postgres"},
    {"name": "g2", "langs": "swift kotlin bash css dart python rust json"},
    {"name": "g3", "langs": "tsql csharp ruby javascript java html xml yaml"},
    {"name": "g4", "langs": "powershell cpp typescript tsx php r golang c"},
]
RUNNERS = [
    {"runner": "ubuntu-24.04", "goos": "linux", "goarch": "amd64"},
    {"runner": "ubuntu-24.04-arm", "goos": "linux", "goarch": "arm64"},
]

# (class, regular expression); the first rule whose expression matches the whole path wins.
RULES = [
    ("lib", r"tools/lane/(ci_changes|test_ci_changes)\.py"),  # a change to the classifier runs everything
    # A change to a required workflow runs that whole workflow (see decide); other workflows (bench, compare, nightly,
    # upstream-watch, ...) are not PR-required and run nothing the required lanes read.
    ("wf:engine", r"\.github/workflows/engine\.yml"),
    ("wf:regen", r"\.github/workflows/regen\.yml"),
    ("wf:conformance", r"\.github/workflows/conformance\.yml"),
    ("wf:windows", r"\.github/workflows/windows\.yml"),
    ("wf:api-guard", r"\.github/workflows/api-guard\.yml"),
    ("wf:pin-move", r"\.github/workflows/pin-move\.yml"),
    ("wf:other", r"\.github/workflows/[^/]+\.ya?ml"),
    ("license", r"LICENSE[^/]*|LICENSES/.*|THIRD_PARTY_NOTICES\.md|grammars/[^/]+/LICENSE[^/]*"),
    ("grammars", r"grammars/.*"),  # including generated README.md files: regen compares their bytes
    ("root", r"go\.mod|go\.sum|[^/]+\.go|internal/.*|experimental/.*|testdata/.*"),
    # Markdown at the root, under docs/, and directly in a tools/<name>/ directory. Deeper markdown under tools/ is test input.
    ("docs", r"[^/]+\.md|docs/.*|artifacts/.*|tools/[^/]+/[^/]+\.md"),
    ("gen", r"tools/gen/.*"),
    ("conf", r"tools/conformance/.*"),
    ("nativeref", r"tools/native-ref/.*"),
    ("bench", r"tools/bench/.*"),
    ("lane", r"tools/lane/.*"),
    ("apiguard", r"tools/apiguard/.*"),
    ("tools-other", r"tools/(compare|modulezip)/.*"),  # no required lane builds or runs them
]


def classify(path):
    for cls, pattern in RULES:
        if re.fullmatch(pattern, path):
            return cls
    return "unknown"


# Classes that make each job required. Anything not listed for a job does not need it.
#   code = every class below whose change can alter what a lane builds or runs.
CODE = {"root", "grammars", "gen", "conf", "nativeref", "bench"}
NEEDS = {
    # engine
    "engine_core": CODE | {"docs", "license"},  # root tests read docs/design/api-stability.md; tools/gen tests read LICENSES
    "engine_winarm": {"root"},  # native windows/arm64 on a pull request: the coverage windows.yml's product job gave root changes
    "engine_platforms": CODE | {"lane"},  # the engine jobs also run the tools/lane unit tests (windows.yml no longer does)
    "engine_grammars": {"root", "grammars"},  # grammar tests are the root engine plus the module; generator drift is regen's
    "engine_floor": {"root", "grammars"},
    "engine_compat": {"root", "grammars", "lane", "gen"},
    # regen: the generator, its inputs and the ADR-0027 C-behaviour gate (Go engine, native-ref, bench ci, conformance kit)
    "regen": CODE | {"license"},
    # conformance: the full matrix; every other change keeps g1 on amd64 (repository-wide TestKnownGap*)
    "conformance_full": CODE,
    # windows (the product build/vet/test lived here; engine.yml runs it on both Windows architectures)
    "windows_licenses": {"license", "grammars", "gen"},  # LICENSE files, grammars/* list, pins.json licenseSHA256
    "windows_race": {"root"},
    "api_guard": {"root", "apiguard"},
    "pin_move": {"gen", "nativeref"},
}
WORKFLOW_OF = {
    "engine_core": "engine", "engine_platforms": "engine", "engine_winarm": "engine", "engine_grammars": "engine", "engine_floor": "engine",
    "engine_compat": "engine", "regen": "regen", "conformance_full": "conformance",
    "windows_licenses": "windows", "windows_race": "windows", "api_guard": "api-guard", "pin_move": "pin-move",
}


# Every job whose job-level "if" reads needs.changes.outputs: the output keys that must all be "true" for it to run. The gate fails when
# a job listed in "expect" did not succeed, which catches a misspelled output name (it evaluates to '' and skips the job silently).
# test_ci_changes.py compares this table with the "if" of every job in the workflows.
JOB_IF = {
    "engine": {
        "engine-linux-amd64": ["engine_core"], "engine-linux-arm64": ["engine_platforms", "full"],
        "engine-macos-arm64": ["engine_platforms", "full"], "engine-windows-amd64": ["engine_platforms"],
        "engine-windows-arm64": ["engine_winarm"], "engine-go126": ["engine_floor"],
        "engine-consumer-compat": ["engine_compat"], "grammar-linux-amd64": ["engine_grammars"],
        "grammar-linux-arm64": ["engine_grammars", "full"], "grammar-macos-arm64": ["engine_grammars", "full"],
        "grammar-windows-amd64": ["engine_grammars"], "grammar-windows-arm64": ["engine_grammars", "full"],
        "grammar-go126": ["engine_floor"],
    },
    "regen": {"regen-grammar": ["regen"]},
    "conformance": {},
    "windows": {"licenses": ["windows_licenses"], "race": ["windows_race"]},
    "api-guard": {"api-guard": ["api_guard"]},
    "pin-move": {"pin-move": ["pin_move"]},
}


# Matrix jobs whose instance count a dynamic matrix decides: the job name (workflow "name:" with the matrix values filled in) per instance
# and the output that holds the matrix. The gate checks that the successful jobs of the run are exactly the instances listed in
# "expect_matrix": a matrix that comes out smaller (a classifier regression) would otherwise leave every needed job green.
# test_ci_changes.py compares the templates with the "name:" lines of the workflows.
ENGINE_MATRIX_JOBS = {
    "grammar-linux-amd64": ("grammar (ubuntu-24.04, Go 1.27.x, shard %d)", "engine_shards"),
    "grammar-linux-arm64": ("grammar (ubuntu-24.04-arm, Go 1.27.x, shard %d)", "engine_shards"),
    "grammar-macos-arm64": ("grammar (macos-latest, Go 1.27.x, shard %d)", "engine_shards"),
    "grammar-windows-amd64": ("grammar (windows-latest, Go 1.27.x, shard %d)", "engine_shards"),
    "grammar-windows-arm64": ("grammar (windows-11-arm, Go 1.27.x, shard %d)", "engine_shards"),
    "grammar-go126": ("grammar (ubuntu-24.04, Go 1.26.x, shard %d)", "floor_shards"),
}
CONFORMANCE_MATRIX_JOB = "conformance (%s, %s)"  # job conformance-grammar: runner, group name
REGEN_MATRIX_JOB = "regenerate group %s and diff (ubuntu-24.04)"  # job regen-grammar: group name


def partition_ok(root):
    present = sorted(p.name for p in (Path(root) / "grammars").iterdir() if p.is_dir())
    for name, table in (("engine shards", [s["grammars"] for s in ENGINE_SHARDS]),
                        ("conformance groups", [g["langs"] for g in CONFORMANCE_GROUPS]),
                        ("regen groups", [g["langs"] for g in REGEN_GROUPS])):
        listed = sorted(n for entry in table for n in entry.split())
        # The conformance lane spells the Go grammar "go" for the corpus but the directory is grammars/golang.
        if listed != present:
            raise SystemExit("%s differ from grammars/*: listed-only %s, missing %s" % (
                name, sorted(set(listed) - set(present)), sorted(set(present) - set(listed))))


def decide(workflow, files, event, root=None):
    """Returns the output mapping for one workflow. files is a list of paths or None (unknown)."""
    classes = {classify(f) for f in files} if files else {"unknown"}
    everything = event != "pull_request" or "unknown" in classes or "lib" in classes
    own = "wf:" + workflow in classes
    full = everything or own  # the whole workflow, every platform and shard
    out = {"classes": ",".join(sorted(classes)), "full": str(event != "pull_request" or own).lower()}
    for key, needed in NEEDS.items():
        if WORKFLOW_OF[key] == workflow:
            out[key] = str(full or bool(classes & needed)).lower()
    if workflow == "engine":
        # PR/main split: pull requests run ubuntu and windows-latest and one floor shard; main, dispatch and a change to engine.yml run all.
        out["engine_shards"] = json.dumps({"include": ENGINE_SHARDS}, separators=(",", ":"))
        floor = ENGINE_SHARDS if out["full"] == "true" else ENGINE_SHARDS[:1]
        out["floor_shards"] = json.dumps({"include": floor}, separators=(",", ":"))
    expect = [j for j, keys in JOB_IF[workflow].items() if all(out[k] == "true" for k in keys)]
    out["expect"] = json.dumps(expect, separators=(",", ":"))
    if workflow == "engine":
        names = [tmpl % s["shard"] for j, (tmpl, key) in ENGINE_MATRIX_JOBS.items() if j in expect for s in json.loads(out[key])["include"]]
    if workflow == "conformance":
        groups = CONFORMANCE_GROUPS if out["conformance_full"] == "true" else CONFORMANCE_GROUPS[:1]
        runners = RUNNERS if out["conformance_full"] == "true" else RUNNERS[:1]
        include = [dict(r, group=g) for g in groups for r in runners]
        out["conformance_matrix"] = json.dumps({"include": include}, separators=(",", ":"))
        names = [CONFORMANCE_MATRIX_JOB % (e["runner"], e["group"]["name"]) for e in include]
    if workflow == "regen":
        out["regen_matrix"] = json.dumps({"include": [{"group": g} for g in REGEN_GROUPS]}, separators=(",", ":"))  # matrix.group.name, as in conformance
        names = [REGEN_MATRIX_JOB % g["name"] for g in REGEN_GROUPS if "regen-grammar" in expect]
    if workflow in ("engine", "conformance", "regen"):
        out["expect_matrix"] = json.dumps(names, separators=(",", ":"))
    return out


def changed_files(event):
    if event != "pull_request":
        return None
    try:
        subprocess.run(["git", "rev-parse", "--verify", "-q", "HEAD^2"], check=True, capture_output=True)
        res = subprocess.run(["git", "diff", "--name-only", "--no-renames", "-z", "HEAD^1", "HEAD"],
                             check=True, capture_output=True, text=True)
    except (subprocess.CalledProcessError, OSError) as err:
        print("::warning::cannot list the pull request's changed files (%s); running everything" % err, file=sys.stderr)
        return None
    return [f for f in res.stdout.split("\0") if f]


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--workflow", required=True, choices=WORKFLOWS)
    ap.add_argument("--files", help='file with one path per line ("-": stdin); default: git diff of the merge commit')
    ap.add_argument("--event", default=os.environ.get("GITHUB_EVENT_NAME", ""))
    ap.add_argument("--root", default=".")
    args = ap.parse_args(argv)
    partition_ok(args.root)
    if args.files:
        text = sys.stdin.read() if args.files == "-" else Path(args.files).read_text(encoding="utf-8")
        files = [l.strip() for l in text.splitlines() if l.strip()]
    else:
        files = changed_files(args.event)
    out = decide(args.workflow, files, args.event)
    lines = ["%s=%s" % kv for kv in out.items()]
    target = os.environ.get("GITHUB_OUTPUT")
    if target:
        with open(target, "a", encoding="utf-8") as fh:
            fh.write("\n".join(lines) + "\n")
    else:
        print("\n".join(lines))
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a", encoding="utf-8") as fh:
            fh.write("### %s: %s (event %s, %d changed files)\n\n" % (
                args.workflow, out["classes"], args.event or "-", len(files) if files else 0))
            fh.write("".join("- %s: %s\n" % (k, v) for k, v in out.items() if k not in ("classes", "engine_shards", "floor_shards", "conformance_matrix", "regen_matrix", "expect_matrix")))
    return 0


if __name__ == "__main__":
    sys.exit(main())
