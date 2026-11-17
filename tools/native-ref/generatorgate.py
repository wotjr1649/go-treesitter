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
import subprocess
import sys
import tarfile
import time
from urllib.parse import urlparse

import proctree


def sha(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def text_sha(path):
    return hashlib.sha256(Path(path).read_bytes().replace(b"\r\n", b"\n")).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


stop_tree = proctree.stop_tree


def execute(args, cwd, log, env=None, timeout=1800, allowed=(0,)):
    if timeout is None or timeout <= 0:
        raise ValueError("a positive command timeout is required")
    start = time.monotonic()
    proc, code, failure, cleanup = None, None, None, None
    try:
        with log.open("wb") as output:
            try:
                proc = proctree.start(list(map(str, args)), cwd=cwd, env=env,
                                      stdout=output, stderr=subprocess.STDOUT)
                try:
                    code = proc.wait(timeout=timeout)
                except subprocess.TimeoutExpired:
                    code = 124
            except BaseException as error:
                failure = error
            finally:
                if proc is not None:
                    try:
                        stop_tree(proc)
                    except BaseException as error:
                        cleanup = error
    finally:
        receipt = {"command": list(map(str, args)), "cwd": str(cwd), "exit": code,
                   "wallSeconds": time.monotonic() - start, "timeoutSeconds": timeout,
                   "logSHA256": sha(log) if log.exists() else None}
        if failure is not None:
            receipt["error"] = repr(failure)
        if cleanup is not None:
            receipt["cleanupError"] = repr(cleanup)
        save(log.with_suffix(".command.json"), receipt)
    if failure is not None:
        if cleanup is not None:
            failure.add_note(f"child tree cleanup also failed: {cleanup!r}")
        raise failure
    if cleanup is not None:
        raise RuntimeError(f"child tree cleanup failed; retained {log}") from cleanup
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


def check_upstream(code, result, report):
    cases = outcomes(result)
    failures = [{"id": "upstream/" + name, "kind": "upstream", "outcome": value}
                for name, value in cases.items() if not is_passed(value)]
    if code or not cases or result["parse_stats"]["total_parses"] <= 0:
        failures.append({"id": "upstream/command", "kind": "upstream", "exit": code,
                         "error": "reference none requires a nonempty, passing upstream suite"})
    report["differences"].extend(failures)


def swift_query_exception(args, pins, grammar, code, result, raw, cwd, root, report, env):
    exception = grammar.get("upstreamTestException")
    if not exception:
        return False
    step = "tree-sitter test: queries/outline.scm -> test/outline/outline.swift"
    error = ("Error: Query compilation failed\n\nCaused by:\n"
             "    Query error at 1:1. Invalid syntax:\n    import Foundation\n    ^")
    stats = result["parse_stats"]
    cases = outcomes(result)
    matched = (args.lang == "swift" and pins["tools"]["treeSitterCLI"] == exception.get("cli") == "0.27.2"
               and exception.get("step") == step and exception.get("reason") and exception.get("retireWhen")
               and code == 1 and raw.rstrip().endswith(error)
               and stats["successful_parses"] == stats["total_parses"] == 312
               and len(outcomes(result.get("parse_results", []))) == 312
               and all(is_passed(value) and value not in ("Skipped", "Platform") for value in cases.values())
               and not any(value for key, value in result.items() if key.endswith("_failures"))
               and len(result.get("highlight_results", [])) == 8
               and not result.get("tag_results")
               and result.get("query_results") == [{"name": "outline", "children": []}])
    report["upstreamTestException"] = exception
    report["upstreamTestExceptionMatches"] = bool(matched)
    if not matched:
        raise ValueError("Swift upstream-test exception is stale or its exact failure signature changed")
    # The original suite exit and log stay intact; this command checks the query the CLI failed to run.
    execute([args.cli, "query", "queries/outline.scm", "test/outline/outline.swift",
             "--config-path", root / "config.json"], cwd, root / "upstream-outline-query.log", env)
    report["upstreamOutlineQueryExit"] = 0
    report["upstreamOutlineQueryLogSHA256"] = sha(root / "upstream-outline-query.log")
    return True


def check_candidate(args, repo, out, grammar, report, env):
    prepared = out / "candidate/work" / ("patches-" + args.lang)
    source = prepared / "first" / grammar["grammarDir"]
    provenance = json.loads((prepared / "provenance.json").read_text(encoding="utf-8"))
    if provenance.get("deterministic") is not True or not provenance.get("outputs"):
        raise ValueError("candidate determinism evidence missing")
    for name, expected in provenance["outputs"].items():
        if sha(source / name) != expected:
            raise ValueError(f"candidate generation output identity drift: {name}")
    module = repo / "grammars" / args.lang
    product = json.loads((module / "provenance.json").read_text(encoding="utf-8"))
    if product.get("grammarGeneration") != provenance or product["identity"]["GrammarRev"] != grammar["commit"]:
        raise ValueError("product module and native candidate generation identities differ")
    for name in ("engine_gen.go", "engine_gen.dat"):
        if sha(module / name) != product["outputs"][name]:
            raise ValueError(f"product module output identity drift: {name}")
    report["candidateGeneration"] = provenance
    report["candidateGenerationProvenanceSHA256"] = sha(prepared / "provenance.json")
    report["productModuleFilesSHA256"] = {path.relative_to(module).as_posix(): sha(path)
                                          for path in sorted(module.rglob("*")) if path.is_file()}
    report["productModuleFileCount"] = len(report["productModuleFilesSHA256"])
    report["candidateGenerationOutputFileCount"] = len(provenance["outputs"])
    execute(["go", "run", ".", "schema", "check", source / "node-types.json"], repo / "tools/gen",
            out / "candidate-schema.log", env)
    assessment = json.loads((out / "candidate-schema.log").read_text(encoding="utf-8"))
    report["candidateSchema"] = assessment
    if assessment.get("assessment") != "PASS" or assessment["input"]["sha256"] != sha(source / "node-types.json"):
        raise ValueError("candidate strict schema check failed")


def product_conformance(args, repo, out, pins, lang, data, tests, label, report, env):
    evidence = out / label
    evidence.mkdir()
    tags = ",".join("no_" + ("go" if name == "golang" else name)
                    for name in sorted(pins["grammars"]) if name != args.lang)
    product_env = env | {"CGO_ENABLED": "0", "GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "local",
                         "GOFLAGS": "", "TSG_DATA_DIR": str(data), "TSG_S15_DIR": "", "TSG_S15_LANGS": lang,
                         "TSG_NATIVE_EXE_DIR": str(out / "candidate/exe"), "TSG_REQUIRE_FIELD_LOOKUP": "1",
                         "TSG_S15_EDITS_SAMPLE": "0", "TSG_CONF_OUT": str(evidence)}
    log = out / (label + ".log")
    execute(["go", "test", "-json", "-count=1", "-timeout", "20m", "-tags", tags,
             "-run", "^(" + "|".join(tests) + ")$", "."], repo / "tools/conformance", log, product_env, timeout=1500)
    events = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines() if line.strip()]
    passed = {event["Test"] for event in events if event.get("Action") == "pass" and "Test" in event}
    skipped = [event["Test"] for event in events if event.get("Action") == "skip" and "Test" in event]
    required = set(tests) | {test + "/" + lang for test in tests}
    if not required <= passed or skipped:
        raise ValueError(f"product checks missing or skipped: {sorted(required - passed)}, skipped={skipped}")
    report[label] = {"tests": list(tests), "passed": sorted(passed), "skipped": skipped,
                     "logSHA256": sha(log), "dataManifestSHA256": sha(data / "corpus/manifest.csv")}


def large_product_data(out, lang, large):
    data = out / "large-product-data"
    (data / "corpus" / lang).mkdir(parents=True)
    (data / "native/dumps" / lang).mkdir(parents=True)
    with (data / "corpus/manifest.csv").open("w", newline="", encoding="utf-8") as stream:
        writer = csv.writer(stream, lineterminator="\n")
        writer.writerow(("id", "lang", "source_ref", "bytes", "sha256"))
        for index, row in enumerate(large):
            name = f"large-{index:04d}"
            source = out / "large" / row["lang"] / row["file"]
            shutil.copyfile(source, data / "corpus" / lang / (name + ".in"))
            shutil.copyfile(out / "candidate" / ("large-" + row["file"] + ".dump"),
                            data / "native/dumps" / lang / (name + ".dump"))
            writer.writerow((lang + "/" + name, lang, "pinned-large/" + row["file"], source.stat().st_size, sha(source)))
    return data


def run(args):
    repo, out, dl = args.repo.resolve(), args.out.resolve(), args.dl.resolve()
    out.mkdir(parents=True, exist_ok=False)
    pinsfile = repo / "tools/gen/pins.json"
    pins = json.loads(pinsfile.read_text(encoding="utf-8"))
    grammar = pins["grammars"][args.lang]
    no_reference = grammar.get("reference") == "none"
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
    if no_reference:
        report.update(reference="none", referenceStatus="NOT_APPLICABLE_NO_COMMITTED_PARSER")
    save(out / "report.json", report)
    try:
        platform = {"win32": "windows", "linux": "linux", "darwin": "darwin"}[sys.platform]
        import platform as host_platform
        arch = {"AMD64": "amd64", "x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[host_platform.machine()]
        report["architecture"] = arch
        compiler = shutil.which(args.cc)
        if not compiler:
            raise ValueError(f"C compiler not found: {args.cc}")
        execute([compiler, "--version"], repo, out / "compiler-version.log", env, timeout=10)
        report["compiler"] = {"command": args.cc, "sha256": sha(compiler),
                              "version": (out / "compiler-version.log").read_text(encoding="utf-8").splitlines()[0]}
        cli_pin = pins["tools"]["treeSitterCLIArchives"][platform + "-" + arch]
        cli_archive = dl / Path(urlparse(cli_pin["url"]).path).name
        if sha(cli_archive) != cli_pin["sha256"]:
            raise ValueError("CLI archive hash mismatch")
        with gzip.open(cli_archive, "rb") as stream:
            if hashlib.file_digest(stream, "sha256").hexdigest() != sha(args.cli):
                raise ValueError("CLI differs from pinned archive")
        execute(["git", "rev-parse", "HEAD"], repo, out / "commit.log", env, timeout=10)
        report["commit"] = (out / "commit.log").read_text(encoding="utf-8").strip()
        report["largeManifestSHA256"] = text_sha(repo / "tools/bench/ci/large.json")
        report["largeDumpTimeoutSeconds"] = args.large_dump_timeout
        reference = out / "reference"
        # The candidate supplies verified lockfile dependencies for both upstream test variants.
        for side, flag in (("candidate", "-candidate"), ("reference", "-reference")):
            if side == "reference" and no_reference:
                continue
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
            if no_reference:
                if (upstream / grammar["grammarDir"] / "parser.c").exists():
                    raise ValueError("reference none contradicts a committed parser.c")
                report["committedParserAbsent"] = True
            elif side == "candidate":
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
            accepted = swift_query_exception(args, pins, grammar, code, report[side + "Upstream"], raw,
                                            cwd, root, report, upstream_env)
            if no_reference and not accepted:
                check_upstream(code, report[side + "Upstream"], report)
            if code and not accepted and all(is_passed(value) for value in outcomes(report[side + "Upstream"]).values()):
                raise ValueError("upstream command failed without a reported failing test")
            if not outcomes(report[side + "Upstream"]):
                if report[side + "Upstream"]["parse_stats"]["total_parses"] != 0:
                    raise ValueError("upstream summary claims parses with no test outcomes")
                report[side + "UpstreamCoverage"] = "NOT_RUN_ZERO_CASES"
            else:
                report[side + "UpstreamCoverage"] = len(outcomes(report[side + "Upstream"]))
            try:
                complete_upstream(args, repo, out, side, grammar, report, env)
            except Exception as error:
                if not no_reference:
                    raise
                report["differences"].append({"id": "upstream/supplement", "kind": "upstream", "error": str(error)})
        input_root = out / "candidate" if no_reference else reference
        report["corpusInputs"] = len(rows(input_root / "data/corpus/manifest.csv"))
        report["seededEdits"] = len(rows(input_root / "data/edits/manifest.csv"))
        if no_reference:
            if not report["corpusInputs"] or not report["seededEdits"]:
                raise ValueError("reference none requires nonempty corpus and edits")
            check_candidate(args, repo, out, grammar, report, env)
            product_conformance(args, repo, out, pins, lang, input_root / "data",
                                ("TestCorpus", "TestEdits", "TestFieldLookup", "TestFieldLookupEdits"),
                                "product-conformance", report, env)
        else:
            report["differences"] = compare_data(reference / "data", out / "candidate/data")
            report["differences"] += compare_upstream(report["referenceUpstream"], report["candidateUpstream"])
        large_data = large_input_data(args, repo, out, pinsfile, lang, env, input_root)
        execute([args.benchci, "fetch", "-manifest", repo / "tools/bench/ci/large.json", "-data", large_data,
                 "-out", out / "large", "-langs", args.lang], repo, out / "large-fetch.log", env)
        large = rows(out / "large/manifest.csv")
        if not large:
            raise ValueError("no pinned large input")
        report["largeInputs"] = large
        report["largeInputCount"] = len(large)
        suffix = ".exe" if os.name == "nt" else ""
        for row in large:
            path = out / "large" / row["lang"] / row["file"]
            if sha(path) != row["sha256"]:
                raise ValueError(f"large input hash mismatch: {path}")
            logs = []
            roots = (out / "candidate",) if no_reference else dict.fromkeys((reference, out / "candidate"))
            for root in roots:
                log = root / ("large-" + row["file"] + ".dump")
                execute([root / "exe" / (args.lang + suffix), "dump", path], repo, log, env,
                        timeout=args.large_dump_timeout)
                logs.append(log)
            if len(logs) == 2 and logs[0].read_bytes() != logs[1].read_bytes():
                report["differences"].append({"id": "large/" + row["file"], "kind": "large", "inputSHA256": row["sha256"]})
        if no_reference:
            data = large_product_data(out, lang, large)
            product_conformance(args, repo, out, pins, lang, data, ("TestCorpus",), "product-large", report, env)
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
