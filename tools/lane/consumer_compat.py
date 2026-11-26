"""Exercise published grammar modules with a candidate root and record the resolved inputs."""

import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

REPO = Path(__file__).resolve().parents[2]
FIXTURES = REPO / "testdata" / "consumer-compat"
ROOT = "github.com/wotjr1649/go-treesitter"
PUBLIC_PROXY = "https://proxy.golang.org"
# Go's semantic version grammar: no leading zeros, no empty identifiers, no build metadata.
_NUM = r"(?:0|[1-9][0-9]*)"
_PRE = r"(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
SEMVER = re.compile(rf"v{_NUM}\.{_NUM}\.{_NUM}(?:-{_PRE}(?:\.{_PRE})*)?")

# (name, tag, ABI, tag commit, minimum root). The tag's go.mod and language.go supply its minimum root and embedded ABI;
# the minimum root equals the tag for these rows but not in general (an RC root can sit under a lower grammar tag).
GRAMMARS = {
    "abi1-json": ("json", "v0.1.0", 1, "2b0c9ec9996e50ed63313f6da5d4ddec98d2b70d", "v0.1.0"),
    "abi2-json": ("json", "v0.2.0", 2, "b4af8ceed73af69796578191e7574dc07f7926bb", "v0.2.0"),
    "abi2-cpp": ("cpp", "v0.3.0", 2, "ee25291cbff710a1c8174592a71aa7dcfcfa19c2", "v0.3.0"),
    "abi3-cpp": ("cpp", "v0.4.0", 3, "eb1fbd5e3da81e648061b19cffd50aa846c8ea82", "v0.4.0"),
    "abi4-go": ("golang", "v0.6.0", 4, "a8af4360e4964deb6c0bb62461ee8348be1aa738", "v0.6.0"),
}
ENV = dict(
    os.environ, CGO_ENABLED="0", GOENV="off", GOWORK="off", GOTOOLCHAIN="local",
    GOOS="", GOARCH="", GOFLAGS="", GOAUTH="off", GOPRIVATE="", GONOPROXY="",
    GONOSUMDB="", GOSUMDB="sum.golang.org", GOPROXY=PUBLIC_PROXY,
)


def run(cwd, env, *args, capture=False):
    print(f"[{cwd}] {subprocess.list2cmdline(args)}", flush=True)
    result = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.PIPE if capture else None)
    print(f"exit {result.returncode}", flush=True)
    result.check_returncode()
    return result.stdout


def isolated(scratch, name):
    cache = scratch / name
    cache.mkdir()
    return dict(ENV, GOPATH=str(cache), GOMODCACHE=str(cache / "mod"))


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def grammar_path(name):
    return f"{ROOT}/grammars/{name}"


def candidate_version(base, commit):
    """Sort just above BASE's own release and below its next one: v0.7.0-consumercompat.X, v1.0.0-rc.1.consumercompat.X."""
    if not SEMVER.fullmatch(base):
        raise ValueError(f"CANDIDATE_BASE {base!r} is not a semantic version")
    # Only a dot keeps an RC base's own identifiers in front: rc.1.consumercompat.X < rc.2 but rc.1-consumercompat.X > rc.2.
    return f"{base}{'.' if '-' in base else '-'}consumercompat.{commit}"


def candidate_zip(scratch, evidence):
    commit = run(REPO, ENV, "git", "rev-parse", "HEAD", capture=True).decode().strip()
    version = candidate_version(os.environ.get("CANDIDATE_BASE", "v0.7.0"), commit)
    proxy = scratch / "proxy"
    versions = proxy / ROOT / "@v"
    versions.mkdir(parents=True)
    mod = versions / f"{version}.mod"
    zip_path = versions / f"{version}.zip"
    mod.write_bytes(run(REPO, ENV, "git", "show", f"{commit}:go.mod", capture=True))
    time = run(REPO, ENV, "git", "show", "-s", "--format=%cI", commit, capture=True).decode().strip()
    (versions / f"{version}.info").write_text(json.dumps({"Version": version, "Time": time}), encoding="utf-8")
    zip_repo = REPO
    if (REPO / ".git").is_file():
        # x/mod's VCS detection needs a .git directory. Share Git objects when in a linked worktree.
        zip_repo = scratch / "modulezip-repo"
        run(scratch, ENV, "git", "clone", "--shared", "--no-checkout", "--no-tags", str(REPO), str(zip_repo))
    run(REPO / "tools" / "modulezip", isolated(scratch, "zip-tool"),
        "go", "run", "-mod=readonly", ".", "-version", version, ROOT, str(zip_repo), commit, "", str(zip_path))

    # Seed only the private candidate's sums. Published modules still use the public sum DB.
    env = dict(isolated(scratch, "candidate-sums"), GOPROXY=proxy.as_uri(), GOSUMDB="off")
    module = json.loads(run(scratch, env, "go", "mod", "download", "-json", f"{ROOT}@{version}", capture=True))
    sums = f"{ROOT} {version} {module['Sum']}\n{ROOT} {version}/go.mod {module['GoModSum']}\n"
    pins = run(REPO, ENV, "git", "show", f"{commit}:tools/gen/pins.json", capture=True)
    identity = {
        "commit": commit, "version": version, "pinsSHA256": hashlib.sha256(pins).hexdigest(),
        "zipSHA256": sha(zip_path), "modSHA256": sha(mod), "goSum": sums.splitlines(),
    }
    print(json.dumps(identity, indent=2), flush=True)
    (evidence / "candidate.json").write_text(json.dumps(identity, indent=2) + "\n", encoding="utf-8")
    shutil.copyfile(mod, evidence / "candidate.mod")
    return version, sums, f"{proxy.as_uri()},{PUBLIC_PROXY}"


def check_graph(cwd, env, out, expected):
    graph = run(cwd, env, "go", "list", "-mod=readonly", "-m", "-json", "all", capture=True)
    out.write_bytes(graph)
    decoder = json.JSONDecoder()
    text = graph.decode().lstrip()
    selected = {}
    while text:
        module, end = decoder.raw_decode(text)
        text = text[end:].lstrip()
        if "Replace" in module:
            raise ValueError(f"replacement in module graph: {module}")
        if not module.get("Main"):
            selected[module["Path"]] = module["Version"]
    if any(selected.get(path) != version for path, version in expected.items()):
        raise ValueError(f"selected {selected}, want {expected}")


def consumer(scratch, label, initial_root, rows):
    cwd = scratch / label
    cwd.mkdir()
    for name in ["consumer", *(row[0] for row in rows)]:
        filename = f"{name}_test.go"
        shutil.copyfile(FIXTURES / filename, cwd / filename)
    (cwd / "go.mod").write_text(
        f"module example.com/consumer-compat\n\ngo 1.26.0\n\nrequire {ROOT} {initial_root}\n",
        encoding="utf-8",
    )
    return cwd


def exercise(cwd, env, out, root, rows, cross=False):
    out.mkdir()
    shutil.copyfile(cwd / "go.mod", out / "go.mod.before")
    try:
        run(cwd, env, "go", "mod", "tidy")
        check_graph(cwd, env, out / "graph.json", {
            ROOT: root, **{grammar_path(name): version for name, version, _, _, _ in rows},
        })
        for name, version, abi, commit, min_root in rows:
            path = grammar_path(name)
            module = json.loads(run(cwd, env, "go", "mod", "download", "-json", f"{path}@{version}", capture=True))
            go_mod = json.loads(run(cwd, env, "go", "mod", "edit", "-json", module["GoMod"], capture=True))
            source = Path(module["Dir"])
            provenance = (source / "provenance.json").read_bytes()
            (out / f"{name}.provenance.json").write_bytes(provenance)
            identity = {
                "module": path, "tag": f"grammars/{name}/{version}", "origin": module.get("Origin"),
                "zipSHA256": sha(Path(module["Zip"])), "modSHA256": sha(Path(module["GoMod"])),
                "sum": module["Sum"], "modSum": module["GoModSum"],
                "abi": abi, "requirements": go_mod["Require"],
            }
            (out / f"{name}.json").write_text(json.dumps(identity, indent=2) + "\n", encoding="utf-8")
            if module.get("Origin", {}).get("Hash") != commit:
                raise ValueError(f"{path}@{version}: expected tag commit {commit}, got {module.get('Origin')}")
            embedded = re.search(r"(?m)^\s*ABI:\s*(\d+),", (source / "language.go").read_text(encoding="utf-8"))
            if not embedded or int(embedded[1]) != abi or json.loads(provenance)["abi"] != abi:
                raise ValueError(f"{path}@{version}: expected ABI {abi}")
            if go_mod.get("Replace") or go_mod.get("Require") != [{"Path": ROOT, "Version": min_root}]:
                raise ValueError(f"{path}@{version}: expected root minimum {min_root} without replacements")
            env[f"TSG_{name.upper()}_ABI"] = str(abi)
        run(cwd, env, "go", "test", "-mod=readonly", "-v", "-count=1", "-timeout=90s", ".")
        if cross:
            print("Cross-link with GOOS=linux GOARCH=arm64", flush=True)
            run(cwd, dict(env, GOOS="linux", GOARCH="arm64"), "go", "test", "-mod=readonly",
                "-c", "-o", str(cwd / "consumer-linux-arm64.test"), ".")
    finally:
        for name in ("go.mod", "go.sum"):
            path = cwd / name
            if path.exists():
                shutil.copyfile(path, out / name)


def main(evidence, scratch):
    version = run(REPO, ENV, "go", "version", capture=True).decode().strip()
    print(version, flush=True)
    if version.split()[2] != "go1.26.0":
        raise ValueError(f"expected Go 1.26.0, got {version}")
    (evidence / "platform.json").write_bytes(run(
        REPO, ENV, "go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED",
        "GOWORK", "GOENV", "GOTOOLCHAIN", "GOPROXY", "GOSUMDB", capture=True,
    ))
    shutil.copytree(FIXTURES, evidence / "fixtures")
    candidate, sums, proxy = candidate_zip(scratch, evidence)

    failures = []
    for label, row in GRAMMARS.items():
        cwd = consumer(scratch, label, candidate, [row])
        env = dict(isolated(scratch, f"{label}-cache"), GOPROXY=proxy)
        name, tag = row[:2]
        run(cwd, env, "go", "mod", "edit", f"-require={grammar_path(name)}@{tag}")
        (cwd / "go.sum").write_text(sums, encoding="utf-8")
        try:
            exercise(cwd, env, evidence / label, candidate, [row])
        except (subprocess.CalledProcessError, ValueError) as exc:
            print(f"{label}: {exc}", flush=True)
            failures.append(label)

    rows = [GRAMMARS["abi1-json"], GRAMMARS["abi4-go"]]
    cwd = consumer(scratch, "mixed", rows[0][4], rows)
    base = isolated(scratch, "mixed-published-cache")
    json_path, go_path = grammar_path("json"), grammar_path("golang")
    run(cwd, base, "go", "mod", "edit", f"-require={json_path}@v0.1.0")
    run(cwd, base, "go", "mod", "download", f"{ROOT}@{rows[0][4]}", f"{json_path}@v0.1.0")
    check_graph(cwd, base, evidence / "mixed-initial-graph.json", {ROOT: rows[0][4], json_path: "v0.1.0"})
    run(cwd, base, "go", "mod", "edit", f"-require={go_path}@v0.6.0")
    try:
        exercise(cwd, base, evidence / "mixed-published", rows[1][4], rows)
    except (subprocess.CalledProcessError, ValueError) as exc:
        print(f"mixed-published: {exc}", flush=True)
        failures.append("mixed-published")
    env = dict(isolated(scratch, "mixed-candidate-cache"), GOPROXY=proxy)
    run(cwd, env, "go", "mod", "edit", f"-require={ROOT}@{candidate}")
    (cwd / "go.sum").write_text(sums, encoding="utf-8")
    try:
        exercise(cwd, env, evidence / "mixed-candidate", candidate, rows, cross=True)
    except (subprocess.CalledProcessError, ValueError) as exc:
        print(f"mixed-candidate: {exc}", flush=True)
        failures.append("mixed-candidate")
    if failures:
        raise SystemExit(f"failed consumers: {', '.join(failures)}")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: consumer_compat.py EVIDENCE_DIR")
    out = Path(sys.argv[1]).resolve()
    out.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="tsg-consumer-") as folder:
        main(out, Path(folder))
