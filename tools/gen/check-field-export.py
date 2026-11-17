"""Compare raw snapshot format-2 exception records across two source trees.

python check-field-export.py --before Q12_TREE --after Q13_TREE --jobs jobs.json --out NEW_DIR
jobs.json: [{"id": "set/lang/name", "lang": "python", "path": "...", "sha256": "..."}, ...]
The output retains the jobs, input identities, both raw tables and build/run logs. No source tree is written.
"""
from pathlib import Path
import argparse
import hashlib
import json
import os
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "native-ref"))
import proctree

LANGS = 'bash c cpp csharp css golang html java javascript json python rust tsql tsx typescript'.split()
PREFIX = 'github.com/wotjr1649/go-treesitter'
GO = r'''package main
import (
 "context"
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "reflect"
 ts "github.com/wotjr1649/go-treesitter"
 IMPORTS
)
type job struct { ID, Lang, Path, SHA256 string }
func main() {
 must := func(err error) { if err != nil { panic(err) } }
 langs := map[string]*ts.Language{LANGUAGES}
 data, err := os.ReadFile(os.Args[1]); must(err)
 var jobs []job; must(json.Unmarshal(data, &jobs))
 parsers := map[string]*ts.Parser{}
 defer func() { for _, p := range parsers { p.Close() } }()
 enc := json.NewEncoder(os.Stdout)
 for _, j := range jobs {
  lang := langs[j.Lang]; if lang == nil { panic("unknown language: " + j.Lang) }
  p := parsers[j.Lang]
  if p == nil { p, err = ts.NewParser(lang); must(err); parsers[j.Lang] = p }
  src, err := os.ReadFile(j.Path); must(err)
  if fmt.Sprintf("%x", sha256.Sum256(src)) != j.SHA256 { panic("input identity drift: " + j.ID) }
  snap, err := p.ParseSnapshot(context.Background(), src); must(err)
  fields := reflect.ValueOf(snap).Elem().FieldByName("fieldLookups")
  if !fields.IsValid() { panic("format-2 fieldLookups slice missing") }
  raw := make([]byte, 0, 12 * fields.Len())
  for i := 0; i < fields.Len(); i++ {
   f := fields.Index(i)
   raw = binary.LittleEndian.AppendUint32(raw, uint32(f.FieldByName("node").Int()))
   raw = binary.LittleEndian.AppendUint32(raw, uint32(f.FieldByName("field").Uint()))
   raw = binary.LittleEndian.AppendUint32(raw, uint32(f.FieldByName("target").Int()))
  }
  must(enc.Encode(struct { ID, SHA256 string; Nodes int; Records string }{j.ID, j.SHA256, snap.Len(), hex.EncodeToString(raw)}))
 }
}
'''


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def identity(root):
    paths = [root / 'tools/gen/pins.json', root / 'tools/gen/shim/tsg_abi.c']
    paths += list(root.glob('*.go')) + list((root / 'internal/engine').glob('*.go'))
    for lang in LANGS:
        paths += list((root / 'grammars' / lang).glob('*'))
    return {p.relative_to(root).as_posix(): sha(p) for p in paths if p.is_file()}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    for flag in ['before', 'after', 'jobs', 'out']:
        ap.add_argument('--' + flag, required=True, type=Path)
    args = ap.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    jobs = args.jobs.read_bytes()
    parsed = json.loads(jobs)
    if not parsed or len({j['id'] for j in parsed}) != len(parsed):
        raise ValueError('jobs must be nonempty with unique IDs')
    (out / 'jobs.json').write_bytes(jobs)
    env = dict(os.environ, CGO_ENABLED='0', GOWORK='off', GOTOOLCHAIN='local')
    receipt = {'role': 'Q12/Q13 exact exception-table differential', 'jobsSHA256': sha(out / 'jobs.json'),
               'inputs': len(parsed), 'go': proctree.run(['go', 'version'], text=True, capture_output=True, check=True, timeout=60).stdout.strip(), 'arms': {}}
    (out / 'receipt.json').write_text(json.dumps(receipt, indent=2), encoding='utf-8')
    for arm in ['before', 'after']:
        root = getattr(args, arm).resolve()
        work = out / arm
        work.mkdir()
        before = identity(root)
        mod = 'module fieldexportcheck\n\ngo 1.26.0\n\nrequire (\n' + PREFIX + ' v0.0.0\n'
        mod += ''.join(PREFIX + '/grammars/' + lang + ' v0.0.0\n' for lang in LANGS) + ')\n'
        mod += 'replace ' + PREFIX + ' => ' + root.as_posix() + '\n'
        mod += ''.join('replace ' + PREFIX + '/grammars/' + lang + ' => ' + (root / 'grammars' / lang).as_posix() + '\n' for lang in LANGS)
        (work / 'go.mod').write_text(mod, encoding='utf-8', newline='\n')
        source = GO.replace('IMPORTS', '\n'.join(('gjson' if lang == 'json' else lang) + ' "' + PREFIX + '/grammars/' + lang + '"' for lang in LANGS))
        source = source.replace('LANGUAGES', ','.join('"' + lang + '":' + ('gjson' if lang == 'json' else lang) + '.Language()' for lang in LANGS) + ',"go":golang.Language(),')
        (work / 'main.go').write_text(source, encoding='utf-8', newline='\n')
        exe = work / 'check.exe'
        print('build/run ' + arm, flush=True)
        with (work / 'build.log').open('wb') as log:
            proctree.run(['go', 'build', '-o', str(exe), '.'], cwd=work, env=env, stdout=log, stderr=subprocess.STDOUT, check=True, timeout=1800)
        with (out / (arm + '.jsonl')).open('wb') as data, (work / 'run.log').open('wb') as log:
            proctree.run([str(exe), str(out / 'jobs.json')], env=env, stdout=data, stderr=log, check=True)
        if before != identity(root):
            raise RuntimeError('source changed during differential: ' + arm)
        receipt['arms'][arm] = {'root': str(root), 'sourceSHA256': before, 'executableSHA256': sha(exe),
                                'recordsSHA256': sha(out / (arm + '.jsonl'))}
        (out / 'receipt.json').write_text(json.dumps(receipt, indent=2), encoding='utf-8')
    before = (out / 'before.jsonl').read_bytes()
    after = (out / 'after.jsonl').read_bytes()
    if len(before.splitlines()) != len(parsed) or len(after.splitlines()) != len(parsed):
        raise RuntimeError('missing output rows')
    mismatches = [i for i, (a, b) in enumerate(zip(before.splitlines(), after.splitlines())) if a != b]
    receipt['differentInputs'] = len(mismatches)
    receipt['mismatches'] = [parsed[i]['id'] for i in mismatches]
    receipt['byteIdentical'] = before == after
    (out / 'receipt.json').write_text(json.dumps(receipt, indent=2), encoding='utf-8')
    if mismatches:
        raise RuntimeError('exception tables differ: ' + repr(receipt['mismatches']))
    print('PASS: exact exception records on ' + str(len(parsed)) + ' inputs', flush=True)


if __name__ == '__main__':
    main()
