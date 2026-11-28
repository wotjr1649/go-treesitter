"""Build and run published release combinations with a fresh module cache for each consumer."""

import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

from consumer_compat import ENV, PUBLIC_PROXY, REPO, ROOT, SEMVER as VERSION, grammar_path

PSEUDO_VERSION_SUFFIX = re.compile(r"[.-][0-9]{14}-[0-9a-f]{12}$")
NAME = re.compile(r"[a-z][a-z0-9]*")
MODULE = "example.com/published-consumer"
SMOKE = """
func parse(language *treesitter.Language, source string) (string, error) {
	p, err := treesitter.NewParser(language)
	if err != nil {
		return "", err
	}
	defer p.Close()
	tree, err := p.Parse(context.Background(), []byte(source), nil)
	if err != nil {
		return "", err
	}
	defer tree.Close()
	snapshot, err := tree.Snapshot(context.Background())
	if err != nil {
		return "", err
	}
	root := snapshot.Root()
	if tree.Outcome() != treesitter.AcceptedClean || root.HasError() {
		return "", fmt.Errorf("outcome %v, root %s", tree.Outcome(), root.Kind())
	}
	return root.Kind(), nil
}

func main() {
	failed := false
	for _, c := range cases {
		root, err := parse(c.language(), c.source)
		if err != nil {
			fmt.Printf("FAIL %s: %v\\n", c.name, err)
			failed = true
			continue
		}
		fmt.Printf("PASS %s: %s\\n", c.name, root)
	}
	if failed {
		os.Exit(1)
	}
}
"""


def escape_tag(tag):
    """Go's module cache path form of a version: each uppercase letter becomes '!' and its lowercase."""
    return re.sub("[A-Z]", lambda m: "!" + m[0].lower(), tag)


def version(value):
    if not VERSION.fullmatch(value) or PSEUDO_VERSION_SUFFIX.search(value):
        raise argparse.ArgumentTypeError(f"expected an explicit release version, got {value!r}")
    return value


def combinations(root, profile):
    inherited = profile["inherited"]
    released = profile["released"]
    for name, tag in inherited.items():
        if not NAME.fullmatch(name):
            raise ValueError(f"invalid grammar name {name!r}")
        version(tag)
    for name, spec in released.items():
        if not NAME.fullmatch(name):
            raise ValueError(f"invalid grammar name {name!r}")
        version(spec["version"])
        version(spec["minimum_root"])
    if not inherited:
        raise ValueError("a combination needs inherited grammar tags")
    samples = profile["samples"]
    needed = {f"{name}@{tag}" for name, tag in inherited.items()} | {f"{name}@{spec['version']}" for name, spec in released.items()}
    if missing := sorted(needed - samples.keys()):
        raise ValueError(f"profile has no sample for {missing}")
    yield "root-only", root, inherited, None
    if released:
        mixed = inherited | {name: spec["version"] for name, spec in released.items()}
        yield "mixed", root, mixed, None
        for name, spec in sorted(released.items()):
            yield f"single-{name}", spec["minimum_root"], {name: spec["version"]}, name


def write_consumer(cwd, expected, grammars, samples):
    requires = "".join(f"\t{path} {tag}\n" for path, tag in expected.items())
    (cwd / "go.mod").write_text(f"module {MODULE}\n\ngo 1.26.0\n\nrequire (\n{requires})\n", encoding="utf-8")
    imports = ['"context"', '"fmt"', '"os"', f'treesitter "{ROOT}"']
    fixtures = []
    for name, tag in sorted(grammars.items()):
        # The input is the Go string literal that the grammar's own tag tested, stored in the release profile.
        sample = samples[f"{name}@{tag}"]
        imports.append(f'g_{name} "{grammar_path(name)}"')
        fixtures.append(f'{{"{name}", g_{name}.Language, {sample}}},')
    source = (
        "package main\n\nimport (\n\t" + "\n\t".join(imports) + "\n)\n\n"
        "var cases = []struct {\n\tname string\n\tlanguage func() *treesitter.Language\n\tsource string\n}{\n\t"
        + "\n\t".join(fixtures) + "\n}\n" + SMOKE
    )
    (cwd / "main.go").write_text(source, encoding="utf-8")


def run(cwd, env, log, *args):
    print(f"{log.parent.name}: {subprocess.list2cmdline(args)}", flush=True)
    proc = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    log.write_bytes(proc.stdout)
    if proc.returncode:
        print(proc.stdout.decode("utf-8", errors="replace"), flush=True)
        proc.check_returncode()
    return proc.stdout.decode("utf-8")


def exercise(scratch, out, env, root, grammars, single, samples):
    out.mkdir()
    cwd = scratch / out.name
    cwd.mkdir()
    cache = scratch / "mod"
    cache.mkdir()  # This temporary directory is new for each consumer.
    env = dict(env, GOPATH=str(scratch), GOMODCACHE=str(cache))
    expected = {ROOT: root, **{grammar_path(name): tag for name, tag in sorted(grammars.items())}}
    try:
        write_consumer(cwd, expected, grammars, samples)
        run(cwd, env, out / "download.log", "go", "mod", "download", "all")
        graph = run(cwd, env, out / "go-list-m-all.txt", "go", "list", "-mod=readonly", "-m", "all")
        selected = {}
        for line in graph.splitlines():
            if line == MODULE:
                continue
            path, *fields = line.split()
            if len(fields) != 1:
                raise ValueError(f"unexpected module selection or replacement: {line}")
            selected[path] = fields[0]
        if any(selected.get(path) != tag for path, tag in expected.items()):
            raise ValueError(f"MVS selected {selected}, expected {expected}")
        # Go ignores a dependency's replace directives, so check each published grammar's own go.mod for them.
        for name, tag in sorted(grammars.items()):
            mod = cache / "cache" / "download" / grammar_path(name) / "@v" / f"{escape_tag(tag)}.mod"
            grammar = json.loads(run(cwd, env, out / f"go-mod-{name}.json", "go", "mod", "edit", "-json", str(mod)))
            if grammar.get("Replace"):
                raise ValueError(f"{name}@{tag}: published go.mod has replace directives")
            if name == single and not any(
                req["Path"] == ROOT and req["Version"] == root for req in grammar.get("Require") or []
            ):
                raise ValueError(f"{single}@{tag}: expected declared minimum root {root}")
        binary = cwd / ("smoke.exe" if sys.platform == "win32" else "smoke")
        run(cwd, env, out / "build.log", "go", "build", "-trimpath", "-mod=readonly", "-o", str(binary), ".")
        run(cwd, env, out / "smoke.log", str(binary))
        run(cwd, env, out / "verify.log", "go", "mod", "verify")
    finally:
        for name in ("go.mod", "go.sum", "main.go"):
            if (cwd / name).is_file():
                shutil.copyfile(cwd / name, out / name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, help="new directory for the resolved graphs, logs and summary")
    parser.add_argument("--root", type=version, default="v1.0.0-rc.1", help="published root for root-only and mixed consumers")
    parser.add_argument("--combination", type=version, default="v1.0.0-rc.1", help="release profile under published-consumers/")
    parser.add_argument("--platform", help="require this native GOOS/GOARCH")
    args = parser.parse_args()
    profile_path = Path(__file__).with_name("published-consumers") / f"{args.combination}.json"
    try:
        profile = json.loads(profile_path.read_text(encoding="utf-8"))
        cases = list(combinations(args.root, profile))
    except (OSError, KeyError, ValueError, argparse.ArgumentTypeError) as exc:
        parser.error(str(exc))

    env = dict(ENV, GO111MODULE="on", GOINSECURE="")
    out = args.evidence.resolve()
    out.mkdir(parents=True)
    names = ("GOVERSION", "GOHOSTOS", "GOHOSTARCH", "GOOS", "GOARCH", "CGO_ENABLED", "GOPROXY", "GOSUMDB",
             "GOWORK", "GOENV", "GOTOOLCHAIN", "GOPRIVATE", "GONOSUMDB", "GONOPROXY")
    platform = json.loads(run(REPO, env, out / "platform.json", "go", "env", "-json", *names))
    native = f"{platform['GOHOSTOS']}/{platform['GOHOSTARCH']}"
    target = f"{platform['GOOS']}/{platform['GOARCH']}"
    if native != target or args.platform and native != args.platform:
        parser.error(f"expected native {args.platform or native}, got host {native}, target {target}")
    shutil.copyfile(profile_path, out / "combination.json")

    failures = []
    with (out / "summary.md").open("w", encoding="utf-8") as summary:
        summary.write(
            f"## Published consumers on {native} ({platform['GOVERSION']})\n\n"
            f"Root {args.root}, combination {args.combination}, proxy {PUBLIC_PROXY}, checksum DB sum.golang.org.\n\n"
            "| Consumer | Root | Grammars | Result |\n"
            "|---|---|---:|---|\n"
        )
        for label, root, grammars, single in cases:
            result = "PASS"
            try:
                with tempfile.TemporaryDirectory(prefix="tsg-pub-") as folder:
                    exercise(Path(folder).resolve(), out / label, env, root, grammars, single, profile["samples"])
            except (OSError, subprocess.CalledProcessError, ValueError) as exc:
                print(f"{label}: {exc}", flush=True)
                failures.append(label)
                result = "FAIL"
            summary.write(f"| {label} | {root} | {len(grammars)} | {result} |\n")
            summary.flush()
    if failures:
        raise SystemExit(f"failed consumers: {', '.join(failures)}")


if __name__ == "__main__":
    main()
