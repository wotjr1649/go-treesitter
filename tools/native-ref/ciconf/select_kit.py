"""Reproduce the bounded kit input manifest: python select_kit.py KIT_ARCHIVE OUT_DIR.

Only routes/gaps source_utf8 and edits are projected; expectations are never read.
"""
import hashlib
import json
import pathlib
import sys
import tarfile

ARCHIVE_SHA256 = "83d792c6453a5f316e101a76885f637b4397ba7a601cfebbd7a388f23cab07d3"
LANGS = ("csharp", "go", "json", "python", "tsql", "tsx", "typescript")


def sha(data):
    return hashlib.sha256(data).hexdigest()


def select(archive, out):
    if sha(archive.read_bytes()) != ARCHIVE_SHA256:
        raise ValueError("kit archive hash mismatch")
    out.mkdir(parents=True, exist_ok=False)
    blobs, cases = {}, []
    with tarfile.open(archive) as tar:
        members = {m.name.split("/", 1)[1]: m for m in tar.getmembers() if "/" in m.name}
        notice = tar.extractfile(members["LICENSE"]).read()
        for lang in LANGS:
            buckets = []
            for group in ("routes", "gaps"):
                path = f"src/testdata/native/{group}/{lang}.json"
                if path not in members:
                    continue
                raw = tar.extractfile(members[path]).read()
                entries = json.loads(raw)["cases"]
                for edited in (True, False):
                    bucket = [c for c in entries if "sample" not in c and bool(c.get("edits")) == edited]
                    bucket.sort(key=lambda c: (sha(c["id"].encode()), c["id"]))
                    buckets.append([(path, sha(raw), c) for c in bucket])
            seen = set()
            count = 0
            while any(buckets) and count < 24:
                for bucket in buckets:
                    if not bucket or count == 24:
                        continue
                    path, file_sha, c = bucket.pop(0)
                    src = c["source_utf8"].encode("utf-8")
                    # Do not lose distinct edits when identical sources are deduplicated.
                    key = (sha(src), json.dumps(c.get("edits", []), sort_keys=True))
                    if key in seen:
                        continue
                    seen.add(key)
                    projected = {"lang": lang, "id": c["id"], "file": path,
                                 "fileSHA256": file_sha, "sourceSHA256": sha(src), "edits": []}
                    blobs[sha(src)] = src
                    for e in c.get("edits", []):
                        find, replace = e["find"].encode(), e["replace"].encode()
                        at = src.find(find) if find else len(src)
                        if at < 0:
                            raise ValueError(f"{c['id']}: missing edit target")
                        new = src[:at] + replace + src[at + len(find):]
                        projected["edits"].append({"find": e["find"], "replace": e["replace"],
                                                   "oldSHA256": sha(src), "newSHA256": sha(new)})
                        blobs[sha(new)] = new
                        src = new
                    cases.append(projected)
                    count += 1
    if sum(map(len, blobs.values())) > 262144:
        raise ValueError("256 KiB unique source budget exceeded")
    (out / "LICENSE").write_bytes(notice)
    (out / ".gitattributes").write_text("sources/*.in -text\n", encoding="utf-8", newline="\n")
    (out / "sources").mkdir()
    for digest, data in sorted(blobs.items()):
        (out / "sources" / (digest + ".in")).write_bytes(data)
    manifest = {"kind": "kit-inputs/1", "repository": "https://github.com/wotjr1649/tree-sitter-grammar-kit",
                "commit": "0a12156", "archiveSHA256": ARCHIVE_SHA256, "license": notice.decode(),
                "selection": "24 cases/grammar; routes,gaps x edited,unedited round-robin; SHA256(id),id order; exclude sample; dedup(source hash,edits)",
                "projection": "UTF-8 bytes unchanged; first find in preceding step; empty find appends; one fresh-old/EditFor/reparse pair per edit; fresh inputs dedup(lang,SHA256)",
                "sourceBudgetBytes": 262144, "cases": cases}
    (out / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    print(f"cases={len(cases)} unique_blobs={len(blobs)} bytes={sum(map(len, blobs.values()))}")


if __name__ == "__main__":
    select(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))
