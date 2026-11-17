#!/usr/bin/env python3
"""Review changed grammar pins with the current schema and native-reference tools."""
import argparse
from collections import Counter
import csv
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import subprocess
import sys
import tarfile
import time


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tools/native-ref"))
import proctree
NODE = re.compile(rb"([0-9]+)\t([^\t]+)\t(.*?)\t([01])\t([01])\t([01])\t([01])\t([01])\t([0-9]+)\t([0-9]+)\t([0-9]+:[0-9]+)\t([0-9]+:[0-9]+)\n", re.DOTALL)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def changed_grammars(old, new):
    a, b = old["grammars"], new["grammars"]
    return [(name, "ADDED" if name not in a else "REMOVED" if name not in b else "CHANGED")
            for name in sorted(a.keys() | b.keys()) if a.get(name) != b.get(name)]


def commits(repo, base, head, merge_base=False):
    """Resolve both refs; with merge_base, compare from the merge base so a stale branch is not judged against pins it lacks."""
    def git(*args):
        done = proctree.run(["git", *args], cwd=repo, capture_output=True, text=True, timeout=120)
        out = done.stdout.strip()
        if done.returncode != 0 or not re.fullmatch(r"[0-9a-f]{40}", out):
            raise ValueError(f"git {args[0]} failed or returned no commit: {done.stderr.strip()}")
        return out
    base, head = (git("rev-parse", "--verify", "--end-of-options", ref + "^{commit}") for ref in (base, head))
    # Fails closed (no merge base, or shallow history) rather than falling back to the moved base tip.
    return (git("merge-base", base, head) if merge_base else base), head


def pins_at(repo, commit):
    done = proctree.run(["git", "show", f"{commit}:tools/gen/pins.json"], cwd=repo, capture_output=True, timeout=120)
    if done.returncode != 0:
        raise ValueError(f"cannot read tools/gen/pins.json at {commit}")
    return json.loads(done.stdout)


def dump_error(data):
    first, separator, body = data.partition(b"\n")
    header = re.fullmatch(rb"#tsg-dump/1 root_has_error=([01]) nodes=([0-9]+)", first)
    if not header or not separator or int(header[2]) == 0:
        raise ValueError("missing, malformed or truncated native dump")
    previous_depth, offset = -1, 0
    # Native token names are unescaped and can contain tabs or newlines (Dart's script tag).
    for i in range(int(header[2])):
        node = NODE.match(body, offset)
        if node is None:
            raise ValueError("malformed native node")
        fields = node.groups()
        depth = int(fields[0])
        if ((i == 0 and depth != 0) or (i > 0 and (depth == 0 or depth > previous_depth + 1))
                or int(fields[8]) > int(fields[9])):
            raise ValueError("malformed native tree")
        if i == 0 and fields[7] != header[1]:
            raise ValueError("native root/header error disagreement")
        previous_depth = depth
        offset = node.end()
    if offset != len(body):
        raise ValueError("native node count/trailing data mismatch")
    return header[1] == b"1"


def classify(old, new):
    a, b = dump_error(old), dump_error(new)
    if not a and b:
        return "OK_TO_ERROR"
    if a and not b:
        return "ERROR_TO_OK"
    return "IDENTICAL" if old == new else "CHANGED"


def overrides(body):
    allowed = set()
    fence = None
    comment = False
    # Only LF ends a line: str.splitlines() also splits on U+2028, U+0085, VT and FF, which Markdown shows inline.
    for line in body.split("\n"):
        line = line.removesuffix("\r")
        marker = re.match(r"^\s{0,3}(`{3,}|~{3,})", line)
        if marker:
            token = marker[1]
            if fence is None:
                fence = token
            elif token[0] == fence[0] and len(token) >= len(fence) and not line[marker.end():].strip():
                fence = None
            continue
        if fence is None and (comment or "<!--" in line):
            for marker in re.finditer(r"<!--|-->", line):
                comment = marker[0] == "<!--"
            continue
        match = re.fullmatch(r"PIN-MOVE-ALLOW-OK-TO-ERROR ([^\s`<>]+)", line)
        if fence is None and match:
            allowed.add(match[1])
    return allowed


def write(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def write_json(path, value):
    write(path, (json.dumps(value, indent=2, ensure_ascii=True) + "\n").encode())


def safe_path(name):
    p = PurePosixPath(name)
    if p.is_absolute() or not p.parts or any(x in (".", "..") or ":" in x or "\\" in x for x in p.parts):
        raise ValueError(f"unsafe relative path: {name!r}")
    return Path(*p.parts)


class Review:
    def __init__(self, out, cache):
        self.out, self.cache = out, cache
        self.serial = 0
        self.env = dict(os.environ, CGO_ENABLED="0", GOWORK="off", GOENV="off", GOTOOLCHAIN="local")

    def command(self, args, cwd=ROOT, timeout=900, check=True):
        self.serial += 1
        log = self.out / "logs" / f"{self.serial:04d}"
        started = time.monotonic()
        # File-backed output retains partial compiler/native evidence on failure or timeout.
        log.parent.mkdir(parents=True, exist_ok=True)
        code, failure = None, None
        try:
            with log.with_suffix(".stdout").open("wb") as stdout, log.with_suffix(".stderr").open("wb") as stderr:
                try:
                    code = proctree.run(args, cwd=cwd, env=self.env, stdout=stdout,
                                        stderr=stderr, timeout=timeout).returncode
                except subprocess.TimeoutExpired:
                    code = "TIMEOUT"
                except BaseException as error:
                    failure = error
        finally:
            receipt = {"args": [str(x) for x in args], "cwd": str(cwd),
                       "timeout_seconds": timeout, "seconds": time.monotonic() - started, "exit": code}
            if failure is not None:
                receipt["error"] = repr(failure)
            write_json(log.with_suffix(".json"), receipt)
        if failure is not None:
            raise failure
        output = log.with_suffix(".stdout").read_bytes()
        if check and code != 0:
            raise ValueError(f"command failed ({code}); see {log.relative_to(self.out)}.stderr")
        return output, code

    def git(self, *args):
        return self.command(["git", *args], timeout=120)[0]

    def build_tools(self):
        self.tools = {}
        for name, module, package in [("native-ref", "native-ref", "."), ("ciconf", "native-ref", "./ciconf"),
                                      ("gen", "gen", "."), ("benchci", "bench", "./ci")]:
            exe = self.out / "bin" / (name + (".exe" if os.name == "nt" else ""))
            self.command(["go", "build", "-o", exe, package], ROOT / "tools" / module)
            self.tools[name] = exe

    def side(self, ref, pins, lang, side, build=True):
        folder = self.out / lang / side
        g = pins["grammars"][lang]
        # Keep full pin identities and materialize recorded patches from the same selected ref.
        pinfile = folder / "pins.json"
        write_json(pinfile, pins)
        for patch in g.get("patches", []):
            file = patch["file"]
            write(folder / safe_path(file), self.git("show", f"{ref}:tools/gen/{file}"))
        sources = [pins["runtime"], g]
        if build:
            sources.append(pins["wasiLibc"])
        if g.get("regenerate"):
            host = ("windows" if os.name == "nt" else platform.system().lower()) + "-" + ("arm64" if platform.machine().lower() in ("aarch64", "arm64") else "amd64")
            cli = pins["tools"]["treeSitterCLIArchives"][host]
            sources.append({"archive": cli["url"].rsplit("/", 1)[1], "archiveSHA256": cli["sha256"]})
        for source in sources:
            digest = source["archiveSHA256"]
            if not re.fullmatch(r"[0-9a-f]{64}", digest):
                raise ValueError("invalid archive SHA256")
            cached = self.cache / "archives" / digest
            if cached.exists():
                raw = cached.read_bytes()
                if sha(raw) != digest:
                    raise ValueError(f"corrupt archive cache: {digest}")
                write(folder / "dl" / safe_path(source["archive"]), raw)
        args = [self.tools["native-ref"], "-pins", pinfile, "-dl", folder / "dl", "-work", folder / "work",
                "-out", folder / "exe", "-fetch", "-cc", "gcc"]
        if build:
            args += ["-corpus", folder / "corpus"]
        else:
            args += ["-nobuild"]
        self.command([*args, lang])
        for source in sources:
            archive = folder / "dl" / safe_path(source["archive"])
            if archive.exists():
                raw = archive.read_bytes()
                if sha(raw) != source["archiveSHA256"]:
                    raise ValueError("archive hash mismatch after native build")
                cached = self.cache / "archives" / source["archiveSHA256"]
                if not cached.exists():
                    write(cached, raw)
        archive = folder / "dl" / safe_path(g["archive"])
        member = PurePosixPath(g["grammarDir"], "node-types.json").as_posix()
        schemas = []
        with tarfile.open(archive, "r:gz") as tar:
            for item in tar:
                if item.name.partition("/")[2] == member:
                    if not item.isfile() or item.size > 16 << 20:
                        raise ValueError("invalid node-types archive member")
                    schemas.append(tar.extractfile(item).read())
        if len(schemas) != 1:
            raise ValueError("missing or duplicate node-types archive member")
        schema = folder / "node-types.json"
        write(schema, schemas[0])
        output, code = self.command([self.tools["gen"], "schema", "check", schema], check=False)
        write(folder / "schema-check.json", output)
        if code != 0:
            raise ValueError(f"{lang}/{side}: strict schema check failed")
        if build:
            self.command([self.tools["ciconf"], "-pins", pinfile, "-corpus", folder / "corpus",
                          "-out", folder / "data", "-lang", "go" if lang == "golang" else lang, "-inputs-only"])
        return folder

    def large_inputs(self, lang, data, folder):
        manifest = json.loads((ROOT / "tools/bench/ci/large.json").read_bytes())
        entries = [e for e in manifest if e["lang"] == lang]
        if not entries:
            raise ValueError(f"{lang}: missing large-input baseline in tools/bench/ci/large.json")
        missing = []
        for entry in entries:
            cached = self.cache / "inputs" / entry["sha256"]
            if cached.exists():
                raw = cached.read_bytes()
                if sha(raw) != entry["sha256"]:
                    raise ValueError("corrupt large-input cache")
                write(folder / "large" / lang / safe_path(entry["file"]), raw)
            else:
                missing.append(entry)
        if missing:
            subset = folder / "large-manifest.json"
            write_json(subset, missing)
            self.command([self.tools["benchci"], "fetch", "-manifest", subset, "-data", data,
                          "-out", folder / "large", "-langs", lang])
        paths = []
        for entry in entries:
            path = folder / "large" / lang / safe_path(entry["file"])
            raw = path.read_bytes()
            if sha(raw) != entry["sha256"]:
                raise ValueError(f"{lang}: large input SHA256 mismatch")
            cached = self.cache / "inputs" / entry["sha256"]
            if not cached.exists():
                write(cached, raw)
            paths.append((f"{lang}/large/{entry['file']}", path))
        return paths

    def grammar(self, base, head, old, new, lang, status, allowed):
        row = {"grammar": lang, "status": status, "pass": True}
        if status == "REMOVED":
            return row
        after = self.side(head, new, lang, "new", build=status != "ADDED")
        if status == "ADDED":
            row["schema"] = "PASS (new only)"
            return row
        before = self.side(base, old, lang, "old")
        output, code = self.command([self.tools["gen"], "schema", "diff", before / "node-types.json", after / "node-types.json"], check=False)
        write(self.out / lang / "schema-diff.json", output)
        if code != 0:
            raise ValueError(f"{lang}: invalid schema diff")
        differences = json.loads(output).get("differences", [])
        row["schema_changes"] = len(differences)
        row["schema_codes"] = dict(Counter(d["code"] for d in differences))
        row["schema_risks"] = dict(Counter(d["risk"] for d in differences))
        inputs = []
        for side, folder in [("old", before), ("new", after)]:
            with (folder / "data/corpus/manifest.csv").open(newline="", encoding="utf-8") as file:
                rows = list(csv.DictReader(file))
            if not rows:
                raise ValueError(f"{lang}/{side}: missing corpus baseline")
            with (folder / "data/corpus/skipped.csv").open(newline="", encoding="utf-8") as file:
                row[side + "_skipped"] = list(csv.DictReader(file))
            for item in rows:
                path = folder / "data/corpus" / safe_path(item["id"] + ".in")
                if sha(path.read_bytes()) != item["sha256"]:
                    raise ValueError("corpus manifest hash mismatch")
                inputs.append((f"{lang}/{side}/{Path(item['id']).name}.in", path))
        inputs += self.large_inputs(lang, after / "data", self.out / lang)
        row["large_inputs"] = sum("/large/" in name for name, _ in inputs)
        row["results"] = []
        counts = Counter()
        for name, path in inputs:
            result = {"id": name, "bytes": path.stat().st_size, "sha256": sha(path.read_bytes()), "override": False}
            dumps = []
            try:
                for side, folder in [("old", before), ("new", after)]:
                    exe = folder / "exe" / (lang + (".exe" if os.name == "nt" else ""))
                    dump, code = self.command([exe, "dump", path], timeout=30, check=False)
                    write(self.out / lang / "dumps" / side / safe_path(name + ".dump"), dump)
                    if code != 0:
                        raise ValueError(f"{side} native dump exit={code}")
                    dumps.append(dump)
                result["classification"] = classify(*dumps)
                result["old_dump_sha256"], result["new_dump_sha256"] = map(sha, dumps)
                result["override"] = result["classification"] == "OK_TO_ERROR" and name in allowed
                if result["classification"] == "OK_TO_ERROR" and not result["override"]:
                    row["pass"] = False
            except ValueError as error:
                result.update(classification="UNPARSABLE", error=str(error))
                row["pass"] = False
            counts[result["classification"]] += 1
            row["results"].append(result)
        row["counts"] = dict(counts)
        write_json(self.out / lang / "results.json", row)
        return row


def summarize(report):
    lines = ["# Grammar pin move", "", f"Status: **{report['status']}**", "",
             f"Base: `{report.get('base', '')}` · Head: `{report.get('head', '')}`", "",
             "Native old/new investigation only; static schemas are not a compatibility verdict.", "",
             "| Grammar | Pin | Schema changes | Identical | Changed | ok→error | error→ok | Unparsable | Result |",
             "|---|---|---:|---:|---:|---:|---:|---:|---|"]
    for row in report["grammars"]:
        c = row.get("counts", {})
        lines.append(f"| {row['grammar']} | {row['status']} | {row.get('schema_changes', '—')} | "
                     + " | ".join(str(c.get(key, 0)) for key in ("IDENTICAL", "CHANGED", "OK_TO_ERROR", "ERROR_TO_OK", "UNPARSABLE"))
                     + f" | {'PASS' if row['pass'] else 'FAIL'} |")
    for row in report["grammars"]:
        lines += ["", f"`{row['grammar']}` schema codes: `{row.get('schema_codes', {})}`; risks: `{row.get('schema_risks', {})}`."]
        if "old_skipped" in row:
            lines += ["", f"Upstream corpus exclusions (ciconf attributes): old={len(row['old_skipped'])}, new={len(row['new_skipped'])}; reasons retained in report.json."]
        if row.get("error"):
            lines += ["", "Failure: " + row["error"]]
        for result in row.get("results", []):
            if result["classification"] in ("OK_TO_ERROR", "UNPARSABLE"):
                lines += ["", f"- `{result['id']}`: {result['classification']}; override={result['override']}"]
    if report.get("error"):
        lines += ["", "Failure: " + report["error"]]
    if report.get("unused_overrides"):
        lines += ["", "Unused override lines: " + ", ".join(f"`{x}`" for x in report["unused_overrides"])]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base")
    parser.add_argument("--head")
    parser.add_argument("--merge-base", action="store_true", help="compare from merge-base(base, head)")
    parser.add_argument("--detect", action="store_true",
                        help="print GITHUB_OUTPUT lines base=, head=, changed= and exit")
    parser.add_argument("--out", type=Path)
    parser.add_argument("--cache", type=Path)
    parser.add_argument("--pr-body-file", type=Path)
    args = parser.parse_args()
    if args.detect:
        if not args.base or not args.head:
            parser.error("--detect needs --base and --head")
        try:
            base, head = commits(ROOT, args.base, args.head, args.merge_base)
            changed = bool(changed_grammars(pins_at(ROOT, base), pins_at(ROOT, head)))
        except (ValueError, OSError, subprocess.SubprocessError) as error:
            parser.exit(1, f"pin-move: {error}\n")
        print(f"base={base}\nhead={head}\nchanged={str(changed).lower()}")
        return 0
    if not args.out or not args.cache:
        parser.error("--out and --cache are required")
    out, cache = args.out.resolve(), args.cache.resolve()
    out.mkdir(parents=True, exist_ok=False)
    review = Review(out, cache)
    report = {"kind": "pin-move/1", "status": "FAIL", "grammars": [], "platform": platform.platform(),
              "native_flags": ["wasi-libc ctype", "-fsigned-char", "-DNDEBUG"], "input_timeout_seconds": 30,
              "tool_source_head": "", "script_sha256": sha(Path(__file__).read_bytes())}
    try:
        report["tool_source_head"] = review.git("rev-parse", "HEAD").decode().strip()
        report["tool_diff_sha256"] = sha(review.git("diff", "HEAD", "--", "tools", ".github/workflows/pin-move.yml"))
        if not args.base or not args.head:
            report["status"] = "diff skipped (explicit base and head required)"
        else:
            refs = commits(ROOT, args.base, args.head, args.merge_base)
            pins = [pins_at(ROOT, commit) for commit in refs]
            if args.merge_base:
                report["requested_base"] = args.base
            report.update(base=refs[0], head=refs[1], old_pins_sha256=sha(review.git("show", f"{refs[0]}:tools/gen/pins.json")),
                          new_pins_sha256=sha(review.git("show", f"{refs[1]}:tools/gen/pins.json")))
            allowed = overrides(args.pr_body_file.read_text(encoding="utf-8") if args.pr_body_file else "")
            changes = changed_grammars(*pins)
            if any(status != "REMOVED" for _, status in changes):
                review.build_tools()
                report["tools"] = {name: sha(path.read_bytes()) for name, path in review.tools.items()}
                report["go_version"] = review.command(["go", "version"])[0].decode().strip()
                report["cc_version"] = review.command(["gcc", "--version"])[0].decode().splitlines()[0]
            for lang, status in changes:
                safe_path(lang)
                print(f"{lang}: {status}", flush=True)
                try:
                    row = review.grammar(*refs, *pins, lang, status, allowed)
                except (ValueError, OSError, KeyError, tarfile.TarError) as error:
                    row = {"grammar": lang, "status": status, "pass": False, "error": str(error)}
                report["grammars"].append(row)
                write_json(out / "report.json", report)
            used = {r["id"] for g in report["grammars"] for r in g.get("results", []) if r["override"]}
            report["unused_overrides"] = sorted(allowed - used)
            report["status"] = "PASS" if all(g["pass"] for g in report["grammars"]) else "FAIL"
    except (ValueError, OSError, KeyError) as error:
        report["error"] = str(error)
    finally:
        write_json(out / "report.json", report)
        write(out / "summary.md", summarize(report).encode("utf-8"))
    print(report["status"], flush=True)
    return int(report["status"] == "FAIL")


if __name__ == "__main__":
    raise SystemExit(main())
