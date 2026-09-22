"""Pinned source preparation, offline C builds, and deterministic oracle records.

Only prepare uses the network. build/record never download or install anything.
"""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import urllib.request

from identity import generate, identity

ROOT = Path(__file__).resolve().parents[2]
PINS = ROOT / 'internal/provenance/identities.json'
WORK = ROOT / '.scratch/oracle'
REPOS = {'runtime': 'tree-sitter/tree-sitter', 'go': 'tree-sitter/tree-sitter-go',
         'python': 'tree-sitter/tree-sitter-python',
         'javascript': 'tree-sitter/tree-sitter-javascript',
         'typescript': 'tree-sitter/tree-sitter-typescript',
         'c_sharp': 'tree-sitter/tree-sitter-c-sharp'}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def read_json(path):
    return json.loads(path.read_text(encoding='utf-8'))


def write_new(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.write('\n')


def bounded(path, base=ROOT):
    path = path.resolve()
    if not path.is_relative_to(base.resolve()) or path == base.resolve():
        raise ValueError('path must stay beneath task root')
    return path


def selected(repo, path):
    if path.name.startswith('LICENSE'):
        return True
    if repo == 'runtime':
        return path.parts[0] == 'lib' and path.suffix in ('.c', '.h')
    return (path.parts[0] in ('src', 'common') or
            (repo == 'typescript' and path.parts[:2] in (('tsx', 'src'), ('typescript', 'src')))) and path.suffix in ('.c', '.h', '.json')


def unpack(data, destination, repo):
    """Extract only regular build inputs; reject links/traversal before writes."""
    destination = bounded(destination)
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        files = {}
        seen, total = set(), 0
        for count, member in enumerate(archive, 1):
            total += member.size
            if count > 20000 or total > 256 * 1024 * 1024:
                raise ValueError('source archive exceeds limits')
            path = PurePosixPath(member.name)
            if path.is_absolute() or '..' in path.parts or '\\' in member.name or ':' in member.name:
                raise ValueError('unsafe archive path')
            if not member.isfile():
                if member.isdir():
                    continue
                raise ValueError('non-regular archive member')
            relative = PurePosixPath(*path.parts[1:])
            if not relative.parts or not selected(repo, relative):
                continue
            target = bounded(destination.joinpath(*relative.parts), destination)
            key = relative.as_posix()
            if key.casefold() in seen:
                raise ValueError('duplicate archive path')
            seen.add(key.casefold())
            files[key] = archive.extractfile(member).read()
        if not files:
            raise ValueError('empty source archive selection')
        if destination.exists():
            raise ValueError('source destination already exists; preserve it')
        for name, content in files.items():
            target = destination / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(content)
        return {name: sha(content) for name, content in sorted(files.items())}


def prepare():
    pins = read_json(PINS)
    for name, repo in REPOS.items():
        commit = (pins['oracle']['runtime_commit'] if name == 'runtime'
                  else pins['grammars'][name]['commit'])
        if not re.fullmatch('[0-9a-f]{40}', commit):
            raise ValueError('invalid public source commit')
        lock = WORK / (name + '.json')
        if lock.exists():
            if read_json(lock)['commit'] != commit:
                raise ValueError('cached source epoch changed')
            print('retained', name, commit, flush=True)
            continue
        url = f'https://codeload.github.com/{repo}/tar.gz/{commit}'
        with urllib.request.urlopen(url, timeout=60) as response:
            data = response.read(64 * 1024 * 1024 + 1)
        if len(data) > 64 * 1024 * 1024:
            raise ValueError('download exceeds limit')
        files = unpack(data, WORK / name, name)
        write_new(lock, {'repository': repo, 'commit': commit, 'archive_sha256': sha(data), 'files': files})
        print('prepared', name, commit, len(files), flush=True)


def source_locks():
    pins = read_json(PINS)
    locks = {}
    for name in REPOS:
        lock = read_json(WORK / (name + '.json'))
        expected = pins['oracle']['runtime_commit'] if name == 'runtime' else pins['grammars'][name]['commit']
        if lock['commit'] != expected or lock['repository'] != REPOS[name]:
            raise ValueError('source epoch/repository mismatch')
        actual = {p.relative_to(WORK / name).as_posix() for p in (WORK / name).rglob('*') if p.is_file()}
        if actual != set(lock['files']):
            raise ValueError('source file set changed')
        for path, digest in lock['files'].items():
            if sha(bounded(WORK / name / path, WORK / name).read_bytes()) != digest:
                raise ValueError(f'source changed: {name}/{path}')
        locks[name] = lock
    return locks


def build(output, regenerate=False):
    output = bounded(output)
    if output.exists():
        raise ValueError('build output already exists')
    if regenerate and not shutil.which(os.environ.get('TREE_SITTER_CLI', 'tree-sitter')):
        raise ValueError('tree-sitter generator unavailable; prepared sources unchanged')
    locks = source_locks()
    pins = read_json(PINS)
    compiler = shutil.which(os.environ.get('CC', 'gcc'))
    if not compiler:
        raise ValueError('C compiler unavailable')
    compiler = str(Path(compiler).resolve())
    compiler_hash = sha(Path(compiler).read_bytes())
    compiler_version = subprocess.check_output([compiler, '--version'], timeout=10).decode().splitlines()[0]
    runtime = WORK / 'runtime/lib'
    output.mkdir(parents=True)
    records = {}
    if regenerate:
        for repo in REPOS:
            if repo != 'runtime':
                shutil.copytree(WORK / repo, output / 'generated' / repo)
    for language in pins['grammars']:
        repo = 'typescript' if language == 'tsx' else language
        source = (output / 'generated' if regenerate else WORK) / repo
        if repo == 'typescript':
            source /= language
        source /= 'src'
        artifact = (generate if regenerate else identity)(runtime / 'include/tree_sitter/api.h', source / 'parser.c')
        executable = output / (language + ('.exe' if os.name == 'nt' else ''))
        command = [compiler, *pins['oracle']['build_flags'], '-I' + str(runtime / 'include'),
                   '-I' + str(runtime / 'src'), '-I' + str(source),
                   '-DORACLE_LANGUAGE=tree_sitter_' + language,
                   str(Path(__file__).with_name('driver.c')), str(runtime / 'src/lib.c'), str(source / 'parser.c')]
        if (source / 'scanner.c').exists():
            command.append(str(source / 'scanner.c'))
        command += ['-o', str(executable)]
        subprocess.run(command, check=True, timeout=180)
        records[language] = {**artifact, 'executable': executable.name,
                             'executable_sha256': sha(executable.read_bytes())}
        print('built', language, artifact['abi'], flush=True)
    generated_files = {p.relative_to(output).as_posix(): sha(p.read_bytes())
                       for p in sorted((output / 'generated').rglob('*')) if p.is_file()} if regenerate else {}
    if sha(Path(compiler).read_bytes()) != compiler_hash:
        raise ValueError('compiler changed during build')
    write_new(output / 'build.json', {'schema': 1, 'pins': pins, 'sources': locks, 'generated_sources': generated_files,
              'driver_sha256': sha(Path(__file__).with_name('driver.c').read_bytes()),
              'compiler': compiler_version, 'compiler_sha256': compiler_hash,
              'os': platform.system(), 'arch': platform.machine(), 'grammars': records})


def fixtures(path):
    cases = read_json(path)
    seen = set()
    for case in cases:
        if not re.fullmatch('[A-Za-z0-9_-]+', case['id']) or case['id'] in seen:
            raise ValueError('duplicate/invalid fixture id')
        seen.add(case['id'])
        if ('source' in case) == ('path' in case):
            raise ValueError('fixture needs exactly one source')
        if 'source' in case:
            data = case['source'].encode('utf-8')
        else:
            path = bounded(ROOT / case['path'], ROOT / 'testdata')
            with path.open('rb') as stream:
                data = stream.read(4 * 1024 * 1024 + 1)
        data.decode('utf-8')
        if len(data) > 4 * 1024 * 1024 or sha(data) != case['sha256']:
            raise ValueError('fixture size/hash mismatch: ' + case['id'])
        yield case, data


def record(build_dir, cases_path, output):
    output = bounded(output)
    if output.exists():
        raise ValueError('record output already exists; evidence is immutable')
    build_dir = bounded(build_dir)
    manifest = read_json(build_dir / 'build.json')
    if manifest['pins'] != read_json(PINS):
        raise ValueError('build epoch changed')
    cases = list(fixtures(cases_path))
    manifest_hash = sha((build_dir / 'build.json').read_bytes())
    for case, data in cases:
        artifact = manifest['grammars'][case['language']]
        executable = bounded(build_dir / artifact['executable'], build_dir)
        if sha(executable.read_bytes()) != artifact['executable_sha256']:
            raise ValueError('executable identity changed')
        result = subprocess.run([str(executable)], input=data, capture_output=True, check=True, timeout=15)
        receipt = json.loads(result.stdout)
        if receipt['abi'] != artifact['abi'] or receipt['input_bytes'] != len(data) or not receipt['nodes']:
            raise ValueError('oracle input/ABI mismatch')
        receipt.update(schema=1, fixture=case['id'], language=case['language'],
                       source_sha256=case['sha256'], build_sha256=manifest_hash)
        receipt['nodes_sha256'] = sha(json.dumps(receipt['nodes'], ensure_ascii=False, separators=(',', ':')).encode())
        write_new(output / (case['id'] + '.json'), receipt)
    write_new(output / 'build.json', manifest)
    write_new(output / 'set.json', {'schema': 1, 'cases_sha256': sha(cases_path.read_bytes()),
              'files': {p.name: sha(p.read_bytes()) for p in sorted(output.glob('*.json'))}})
    print('recorded', len(cases), 'fixtures', flush=True)


def compare(left, right, cases_path=ROOT / 'testdata/oracle/cases.json'):
    """Compare complete records without transferring one build's identity."""
    def load(directory):
        directory = bounded(directory)
        index = read_json(directory / 'set.json')
        if index['schema'] != 1 or {p.name for p in directory.iterdir()} != set(index['files']) | {'set.json'}:
            raise ValueError('record inventory changed')
        records = {}
        build_hash = index['files']['build.json']
        for name, digest in index['files'].items():
            path = bounded(directory / name, directory)
            if sha(path.read_bytes()) != digest:
                raise ValueError('record file changed')
            if name == 'build.json':
                continue
            r = read_json(path)
            if (r['build_sha256'] != build_hash or r['schema'] != 1 or
                    r['start'] != 0 or r['end'] != r['input_bytes'] or not r['nodes'] or
                    r['nodes_sha256'] != sha(json.dumps(r['nodes'], ensure_ascii=False, separators=(',', ':')).encode())):
                raise ValueError('invalid or incomplete C receipt')
            if r['fixture'] in records or name != r['fixture'] + '.json':
                raise ValueError('duplicate/invalid C fixture')
            records[r['fixture']] = r
        return index, read_json(directory / 'build.json'), records
    li, lb, lr = load(left)
    ri, rb, rr = load(right)
    cases = {c['id']: c for c, _ in fixtures(cases_path)}
    if (not cases or li['cases_sha256'] != ri['cases_sha256'] or
            li['cases_sha256'] != sha(cases_path.read_bytes()) or lb['pins'] != rb['pins'] or
            lr.keys() != rr.keys() or lr.keys() != cases.keys()):
        raise ValueError('comparison epoch/fixture identity mismatch')
    rows = []
    for name, a in lr.items():
        b = rr[name]
        if a['source_sha256'] != cases[name]['sha256'] or a['language'] != cases[name]['language']:
            raise ValueError('comparison catalog identity mismatch')
        if any(a[key] != b[key] for key in ('language', 'source_sha256', 'input_bytes')):
            raise ValueError('comparison input identity mismatch')
        first = next((i for i, (x, y) in enumerate(zip(a['nodes'], b['nodes'])) if x != y), -1)
        if first < 0 and len(a['nodes']) != len(b['nodes']):
            first = min(len(a['nodes']), len(b['nodes']))
        row = {'fixture': name, 'source_sha256': a['source_sha256'],
               'left_digest': a['nodes_sha256'], 'right_digest': b['nodes_sha256'],
               'equal': first == -1 and a['has_error'] == b['has_error'], 'first_difference': first}
        if first >= 0:
            row.update(left=a['nodes'][first] if first < len(a['nodes']) else None,
                       right=b['nodes'][first] if first < len(b['nodes']) else None)
        rows.append(row)
    print(json.dumps({'left_build': li['files']['build.json'], 'right_build': ri['files']['build.json'],
                      'comparisons': rows}, ensure_ascii=False, indent=2))
    return all(row['equal'] for row in rows)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'build', 'record', 'compare'])
    parser.add_argument('--build', type=Path)
    parser.add_argument('--cases', type=Path, default=ROOT / 'testdata/oracle/cases.json')
    parser.add_argument('--output', type=Path)
    parser.add_argument('--generate', action='store_true', help='regenerate copied grammar JSON with the currently resolved CLI')
    parser.add_argument('--left', type=Path)
    parser.add_argument('--right', type=Path)
    args = parser.parse_args()
    if args.action == 'prepare':
        prepare()
    elif args.action == 'build' and args.output:
        build(args.output, args.generate)
    elif args.action == 'record' and args.build and args.output:
        record(args.build, args.cases, args.output)
    elif args.action == 'compare' and args.left and args.right:
        sys.exit(0 if compare(args.left, args.right, args.cases) else 1)
    else:
        parser.error('build needs --output; record needs --build and --output')
