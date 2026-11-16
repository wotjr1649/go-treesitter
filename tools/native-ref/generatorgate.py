#!/usr/bin/env python3
"""ADR-0027: compare one pinned generator candidate with its committed-C reference.

Build native-ref, ciconf and benchci from this checkout first. Output must be new;
all commands, identities, failing inputs, native dumps and upstream summaries remain there.
"""

import argparse
import csv
import gzip
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import signal
import subprocess
import sys
import tarfile
import time
from urllib.parse import urlparse


def sha(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def text_sha(path):
    return hashlib.sha256(Path(path).read_bytes().replace(b"\r\n", b"\n")).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def stop_tree(proc):
    if os.name == "nt":
        try:
            result = subprocess.run(["taskkill", "/F", "/T", "/PID", str(proc.pid)],
                                    capture_output=True, check=False, timeout=10)
            if result.returncode and proc.poll() is None:
                proc.kill()
                proc.wait(timeout=10)
                raise RuntimeError(f"taskkill failed for child tree {proc.pid}")
        except (OSError, subprocess.TimeoutExpired):
            proc.kill()
            proc.wait(timeout=10)
            raise RuntimeError(f"taskkill did not finish for child tree {proc.pid}")
    else:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=10)


def execute(args, cwd, log, env=None, timeout=1800, allowed=(0,)):
    start = time.monotonic()
    with log.open("wb") as output:
        proc = subprocess.Popen(list(map(str, args)), cwd=cwd, env=env, stdout=output,
                                stderr=subprocess.STDOUT,
                                start_new_session=os.name != "nt",
                                creationflags=subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0)
        code = None
        try:
            try:
                code = proc.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                code = 124
        finally:
            if code is None or code == 124:
                stop_tree(proc)
    receipt = {"command": list(map(str, args)), "cwd": str(cwd), "exit": code,
               "wallSeconds": time.monotonic() - start, "timeoutSeconds": timeout, "logSHA256": sha(log)}
    save(log.with_suffix(".command.json"), receipt)
    if code not in allowed:
        raise RuntimeError(f"command exited {code}; retained {log}")
    return code


def identical_lfs_reference(original, candidate, grammar):
    parser = (original / "parser.c").read_bytes()
    if not parser.startswith(b"version https://git-lfs.github.com/spec/v1\n"):
        return None
    match = re.search(rb"(?m)^oid sha256:([0-9a-f]{64})$", parser)
    if not match:
        raise ValueError("committed LFS parser has no sha256 OID")
    oid = match[1].decode("ascii")
    if grammar.get("patches") or sha(candidate / "parser.c") != oid:
        return None
    # Sharing requires every other C/header input to be byte-identical as well.
    def inputs(root):
        return {str(path.relative_to(root)): sha(path) for path in root.rglob("*")
                if path.is_file() and path.suffix in (".c", ".h") and path != root / "parser.c"}
    return oid if inputs(original) == inputs(candidate) else None


def extract(archive, dest, expected):
    if sha(archive) != expected:
        raise ValueError(f"archive hash mismatch: {archive}")
    dest.mkdir()
    with tarfile.open(archive) as tar:
        top = None
        for entry in tar:
            parts = PurePosixPath(entry.name).parts
            if not parts or ".." in parts or ":" in entry.name or "\\" in entry.name or entry.name.startswith("/"):
                raise ValueError(f"unsafe archive entry: {entry.name}")
            top = top or parts[0]
            if top != parts[0] or not (entry.isfile() or entry.isdir()):
                raise ValueError(f"unsupported archive entry: {entry.name}")
            rel = parts[1:]
            if not rel:
                continue
            target = dest.joinpath(*rel)
            if entry.isdir():
                target.mkdir(parents=True, exist_ok=True)
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                with target.open("xb") as out, tar.extractfile(entry) as source:
                    shutil.copyfileobj(source, out)


def rows(path):
    with path.open(newline="", encoding="utf-8") as stream:
        return list(csv.DictReader(stream))


def compare_data(before, after):
    differences = []
    for rel in ("corpus/manifest.csv", "edits/manifest.csv"):
        if (before / rel).read_bytes() != (after / rel).read_bytes():
            raise ValueError(f"input identity differs: {rel}")
    corpus = rows(before / "corpus/manifest.csv")
    edit_inputs = rows(before / "edits/manifest.csv")
    if not corpus or not edit_inputs:
        raise ValueError("native corpus and seeded edits must both be nonempty")
    if len({row["id"] for row in corpus}) != len(corpus):
        raise ValueError("duplicate native corpus identity")
    for row in corpus:
        rel = Path("native/dumps") / (row["id"] + ".dump")
        if (before / rel).read_bytes() != (after / rel).read_bytes():
            differences.append({"id": "corpus/" + row["id"], "kind": "corpus", "inputSHA256": row["sha256"]})
    a, b = rows(before / "edits/results-native.csv"), rows(after / "edits/results-native.csv")
    identities = [r["id"] for r in edit_inputs]
    if len(set(identities)) != len(identities) or any([r["id"] for r in side] != identities for side in (a, b)):
        raise ValueError("edit results do not cover the input manifest exactly")
    edits = {r["id"]: r for r in edit_inputs}
    for old, new in zip(a, b):
        changed = [key for key in old if key != "edit_ms" and old[key] != new[key]]
        for suffix in ("ranges", "fields"):
            rel = Path("edits") / (old["id"] + "." + suffix)
            if (before / rel).read_bytes() != (after / rel).read_bytes():
                changed.append(suffix)
        if changed:
            differences.append({"id": "edits/" + old["id"], "kind": "edit", "changed": changed,
                                "oldSHA256": edits[old["id"]]["old_sha256"],
                                "newSHA256": edits[old["id"]]["new_sha256"]})
    return differences


def outcomes(value, path=()):
    result = {}
    if isinstance(value, dict):
        if "outcome" in value:
            return {"/".join(path + (value.get("name", ""),)): value["outcome"]}
        for key, child in value.items():
            if key in ("parse_results", "highlight_results", "tag_results", "query_results", "children"):
                result.update(outcomes(child, path + ((value["name"],) if "name" in value else (key,))))
    elif isinstance(value, list):
        for child in value:
            result.update(outcomes(child, path))
    return result


def is_passed(outcome):
    return outcome in ("Passed", "Skipped", "Platform") or (
        isinstance(outcome, dict) and set(outcome) == {"AssertionPassed"})


def compare_upstream(before, after):
    a, b = outcomes(before), outcomes(after)
    if a.keys() != b.keys():
        raise ValueError("upstream test cases changed")
    changed = [{"id": "upstream/" + key, "kind": "upstream", "before": a[key], "after": b[key]}
               for key in a if a[key] != b[key] and not is_passed(b[key])]
    # Existing failing trees must retain the same failure, not only the test name.
    for category in ("parse_failures", "highlight_failures", "tag_failures", "query_failures"):
        old_failures = {x["name"]: x for x in before.get(category, [])}
        for failure in after.get(category, []):
            if failure["name"] in old_failures and failure != old_failures[failure["name"]]:
                changed.append({"id": "upstream/" + category + "/" + failure["name"], "kind": "upstream"})
    return changed


def summary(raw):
    decoder = json.JSONDecoder()
    for start, value in enumerate(raw):
        if value != "{":
            continue
        try:
            parsed, _ = decoder.raw_decode(raw[start:])
        except ValueError:
            continue
        if isinstance(parsed, dict) and "parse_results" in parsed and "parse_stats" in parsed:
            return parsed
    raise ValueError("upstream JSON summary missing (raw log retained)")


def exception_matches(expected, differences):
    return bool(expected) and {item["id"] for item in differences} == set(expected)


def upstream_dependencies(out, upstream, grammar, side, report):
    expected = grammar.get("npmDependencies", [])
    if not expected:
        return
    prepared = out / "candidate/work" / ("patches-" + report["grammar"])
    provenance_file = prepared / "provenance.json"
    provenance = json.loads(provenance_file.read_text(encoding="utf-8"))
    dependencies = provenance.get("npm", [])
    if sorted(item["name"] for item in dependencies) != sorted(expected):
        raise ValueError("upstream npm inputs differ from the generated candidate")
    lock_hash = sha(upstream / "package-lock.json")
    if any(item["scriptsExecuted"] or item["lockfileSHA256"] != lock_hash for item in dependencies):
        raise ValueError("upstream npm provenance mismatch")
    source = prepared / "first" / Path(grammar["grammarDir"]).parent / "node_modules"
    shutil.copytree(source, upstream / "node_modules")
    report[side + "UpstreamNPM"] = dependencies
    report[side + "UpstreamNPMProvenanceSHA256"] = sha(provenance_file)


def complete_upstream(args, repo, out, side, grammar, report, env):
    original = report[side + "Upstream"]
    failures = original.get("parse_failures", [])
    if not failures:
        return
    root = out / side
    upstream = root / "upstream"
    cwd = upstream if (upstream / "test/corpus").is_dir() else upstream / Path(grammar["grammarDir"]).parent
    excluded = "^(?:" + "|".join(re.escape(case["name"]) for case in failures) + ")$"
    command = [args.cli, "test", "--rebuild", "--json-summary", "--config-path", root / "config.json", "--exclude", excluded]
    code = execute(command, cwd, root / "upstream-supplement.log",
                   env | {"TREE_SITTER_LIBDIR": str(root / "lib")}, allowed=(0, 1))
    extra = summary((root / "upstream-supplement.log").read_text(encoding="utf-8"))
    report[side + "UpstreamSupplement"] = extra
    report[side + "UpstreamSupplementExit"] = code
    report[side + "UpstreamExcludedCases"] = [case["name"] for case in failures]
    if code and all(is_passed(value) for value in outcomes(extra).values()):
        raise ValueError("supplemental upstream command failed without a reported failing test")
    # Keep every original corpus result and failure; only recover stages skipped after that failure.
    combined = original.copy()
    for category in ("highlight_results", "tag_results", "query_results"):
        combined[category] = extra.get(category, [])
    for category in ("highlight_failures", "tag_failures", "query_failures"):
        if category in extra:
            combined[category] = extra[category]
    report[side + "UpstreamFullSummary"] = original
    report[side + "Upstream"] = combined
    report[side + "UpstreamCoverage"] = len(outcomes(combined)) or "NOT_RUN_ZERO_CASES"


def large_input_data(args, repo, out, pinsfile, lang, env, reference=None):
    reference = reference or out / "reference"
    manifest = json.loads((repo / "tools/bench/ci/large.json").read_text(encoding="utf-8"))
    if not any(item["lang"] == args.lang and item["kind"] == "concat" for item in manifest):
        return reference / "data"
    # The pinned concat predates kit inputs and is defined over the upstream-only corpus pool.
    projection = out / "large-input-data"
    execute([args.ciconf, "-pins", pinsfile, "-corpus", reference / "corpus", "-out", projection,
             "-lang", lang, "-inputs-only"], repo, out / "large-input-data.log", env)
    return projection


def run(args):
    repo, out, dl = args.repo.resolve(), args.out.resolve(), args.dl.resolve()
    out.mkdir(parents=True, exist_ok=False)
    pinsfile = repo / "tools/gen/pins.json"
    pins = json.loads(pinsfile.read_text(encoding="utf-8"))
    grammar = pins["grammars"][args.lang]
    lang = "go" if args.lang == "golang" else args.lang
    env = os.environ.copy()
    env.update(NO_COLOR="1", TERM="dumb", CGO_ENABLED="0", CC=args.cc)
    report = {"kind": "generator-gate/1", "role": "adoption candidate", "grammar": args.lang,
              "gateScriptSHA256": text_sha(__file__),
              "textIdentityCanonicalization": "CRLF to LF before SHA-256 (pins, gate script, large manifest)",
              "pinsSHA256": text_sha(pinsfile), "grammarCommit": grammar["commit"], "seed": args.seed,
              "platform": sys.platform, "python": sys.version, "buildAndDataRunsPerVariant": 1,
              "upstreamRunPlan": "full suite once; after corpus failure, one supplemental run excluding exactly the failed corpus case names",
              "tools": {name: sha(getattr(args, name)) for name in ("native_ref", "ciconf", "benchci", "cli")},
              "differences": [], "pass": False}
    save(out / "report.json", report)
    try:
        platform = {"win32": "windows", "linux": "linux", "darwin": "darwin"}[sys.platform]
        import platform as host_platform
        arch = {"AMD64": "amd64", "x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[host_platform.machine()]
        report["architecture"] = arch
        compiler = shutil.which(args.cc)
        if not compiler:
            raise ValueError(f"C compiler not found: {args.cc}")
        report["compiler"] = {"command": args.cc, "sha256": sha(compiler),
                              "version": subprocess.check_output([compiler, "--version"], text=True).splitlines()[0]}
        cli_pin = pins["tools"]["treeSitterCLIArchives"][platform + "-" + arch]
        cli_archive = dl / Path(urlparse(cli_pin["url"]).path).name
        if sha(cli_archive) != cli_pin["sha256"]:
            raise ValueError("CLI archive hash mismatch")
        with gzip.open(cli_archive, "rb") as stream:
            if hashlib.file_digest(stream, "sha256").hexdigest() != sha(args.cli):
                raise ValueError("CLI differs from pinned archive")
        report["commit"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repo, text=True).strip()
        report["largeManifestSHA256"] = text_sha(repo / "tools/bench/ci/large.json")
        report["largeDumpTimeoutSeconds"] = args.large_dump_timeout
        reference = out / "reference"
        # The candidate supplies verified lockfile dependencies for both upstream test variants.
        for side, flag in (("candidate", "-candidate"), ("reference", "-reference")):
            if side == "reference" and report.get("referenceReusesCandidate"):
                reference = out / "candidate"
                report.update({"reference" + key[len("candidate"):]: value
                               for key, value in list(report.items()) if key.startswith("candidate")})
                continue
            root = out / side
            root.mkdir()
            execute([args.native_ref, "-pins", pinsfile, "-dl", dl, "-work", root / "work", "-out", root / "exe",
                     "-corpus", root / "corpus", "-cc", args.cc, flag, args.lang], repo, root / "build.log", env)
            execute([args.ciconf, "-pins", pinsfile, "-corpus", root / "corpus", "-exe", root / "exe",
                     "-out", root / "data", "-lang", lang, "-seed", args.seed, "-j", args.jobs,
                     "-kit", repo / "tools/conformance/testdata/kit"], repo, root / "ciconf.log", env)
            source = root / "work" / ("patches-" + args.lang) / "first"
            if not source.is_dir():
                source = root / "work" / grammar["extractDir"]
            suffix = ".exe" if os.name == "nt" else ""
            report[side + "ParserSHA256"] = sha(source / grammar["grammarDir"] / "parser.c")
            report[side + "ExecutableSHA256"] = sha(root / "exe" / (args.lang + suffix))
            upstream = root / "upstream"
            extract(dl / grammar["archive"], upstream, grammar["archiveSHA256"])
            if side == "candidate":
                oid = identical_lfs_reference(upstream / grammar["grammarDir"], source / grammar["grammarDir"], grammar)
                if oid:
                    report["referenceReusesCandidate"] = True
                    report["referenceKind"] = "LFS_OID_IDENTICAL_TO_CANDIDATE"
                    report["referenceLFSOID"] = oid
            # Multi-grammar repositories need their auxiliary parsers as committed upstream.
            shutil.copytree(source / grammar["grammarDir"], upstream / grammar["grammarDir"], dirs_exist_ok=True)
            upstream_dependencies(out, upstream, grammar, side, report)
            (root / "lib").mkdir()
            upstream_env = env | {"TREE_SITTER_LIBDIR": str(root / "lib")}
            config = root / "config.json"
            save(config, {"parser-directories": []})
            cwd = upstream
            if not (cwd / "test/corpus").is_dir():
                cwd = upstream / Path(grammar["grammarDir"]).parent
            code = execute([args.cli, "test", "--rebuild", "--json-summary", "--config-path", config], cwd,
                           root / "upstream.log", upstream_env, allowed=(0, 1))
            raw = (root / "upstream.log").read_text(encoding="utf-8")
            report[side + "Upstream"] = summary(raw)
            report[side + "UpstreamExit"] = code
            if code and all(is_passed(value) for value in outcomes(report[side + "Upstream"]).values()):
                raise ValueError("upstream command failed without a reported failing test")
            if not outcomes(report[side + "Upstream"]):
                if report[side + "Upstream"]["parse_stats"]["total_parses"] != 0:
                    raise ValueError("upstream summary claims parses with no test outcomes")
                report[side + "UpstreamCoverage"] = "NOT_RUN_ZERO_CASES"
            else:
                report[side + "UpstreamCoverage"] = len(outcomes(report[side + "Upstream"]))
            complete_upstream(args, repo, out, side, grammar, report, env)
        report["differences"] = compare_data(reference / "data", out / "candidate/data")
        report["differences"] += compare_upstream(report["referenceUpstream"], report["candidateUpstream"])
        report["corpusInputs"] = len(rows(reference / "data/corpus/manifest.csv"))
        report["seededEdits"] = len(rows(reference / "data/edits/manifest.csv"))
        large_data = large_input_data(args, repo, out, pinsfile, lang, env, reference)
        execute([args.benchci, "fetch", "-manifest", repo / "tools/bench/ci/large.json", "-data", large_data,
                 "-out", out / "large", "-langs", args.lang], repo, out / "large-fetch.log", env)
        large = rows(out / "large/manifest.csv")
        if not large:
            raise ValueError("no pinned large input")
        report["largeInputs"] = large
        suffix = ".exe" if os.name == "nt" else ""
        for row in large:
            path = out / "large" / row["lang"] / row["file"]
            if sha(path) != row["sha256"]:
                raise ValueError(f"large input hash mismatch: {path}")
            logs = []
            for root in dict.fromkeys((reference, out / "candidate")):
                log = root / ("large-" + row["file"] + ".dump")
                execute([root / "exe" / (args.lang + suffix), "dump", path], repo, log, env,
                        timeout=args.large_dump_timeout)
                logs.append(log)
            if len(logs) == 2 and logs[0].read_bytes() != logs[1].read_bytes():
                report["differences"].append({"id": "large/" + row["file"], "kind": "large", "inputSHA256": row["sha256"]})
        exception = grammar.get("generatorException")
        report["exception"] = exception
        report["pass"] = not report["differences"] and not exception
        if exception:
            report["exceptionMatches"] = exception_matches(exception["failingInputs"], report["differences"])
            report["pass"] = report["exceptionMatches"] and not grammar.get("regenerate", False)
        return report["pass"]
    except Exception as error:
        report["error"] = str(error)
        return False
    finally:
        save(out / "report.json", report)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", type=Path, default=Path(__file__).resolve().parents[2])
    for name in ("native-ref", "ciconf", "benchci", "cli", "dl", "out"):
        parser.add_argument("--" + name, type=Path, required=True)
    parser.add_argument("--lang", required=True)
    parser.add_argument("--cc", default="gcc")
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--jobs", type=int, default=1)
    parser.add_argument("--large-dump-timeout", type=float, default=120,
                        help="seconds per pinned large native dump (default: 120)")
    args = parser.parse_args()
    if args.large_dump_timeout <= 0:
        parser.error("--large-dump-timeout must be positive")
    for name in ("native_ref", "ciconf", "benchci", "cli"):
        setattr(args, name, getattr(args, name).resolve(strict=True))
    ok = run(args)
    print(f"generator gate {args.lang}: {'PASS' if ok else 'FAIL'}; {args.out / 'report.json'}")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
