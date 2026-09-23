"""Reproduce the internal runtime from an identified local module archive.

No network, dependency execution, or in-place replacement. Output must be a new
directory inside the repository. Review patches before changing their inventory.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import zipfile
from separate import separate

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
IMPORTS = re.compile(rb'(?m)^import\s+(?:\([^)]*\)|[^\n]+)')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def build(archive, output, optional_output=None):
    source = json.loads((HERE / 'source.json').read_text(encoding='utf-8'))
    pins = json.loads((ROOT / 'internal/provenance/identities.json').read_text(encoding='utf-8'))
    if source['baseline'] != pins['baseline']:
        raise ValueError('baseline identity differs')
    if digest(archive.read_bytes()) != source['archive_sha256']:
        raise ValueError('module archive hash differs')
    output = output.resolve()
    relative = output.relative_to(ROOT).as_posix()
    if output == ROOT or output.exists():
        raise ValueError('output must be a new task-local directory')
    if optional_output is not None:
        optional_output = optional_output.resolve()
        optional_output.relative_to(ROOT)
        if optional_output == ROOT or optional_output.exists() or optional_output == output:
            raise ValueError('optional output must be a different new task-local directory')
    upstream = source['baseline']['module']
    prefix = upstream + '@' + source['baseline']['version'] + '/'
    files = {}
    with zipfile.ZipFile(archive) as zipped:
        if len(set(zipped.namelist())) != len(zipped.namelist()):
            raise ValueError('duplicate archive member')
        for name in zipped.namelist():
            if not name.startswith(prefix):
                raise ValueError('unexpected archive prefix')
            path = PurePosixPath(name[len(prefix):])
            if path.is_absolute() or '..' in path.parts or '\\' in str(path):
                raise ValueError('unsafe archive member')
            selected = (str(path.parent) in source['packages'] and
                        path.suffix == '.go' and not path.name.endswith('_test.go'))
            selected |= str(path) in source['assets']
            selected |= str(path.parent) == 'grammars/grammar_blobs' and path.suffix == '.bin'
            if selected:
                files[str(path)] = zipped.read(name)
    if not files or len(files) != source['file_count']:
        raise ValueError('source inventory differs')
    patches = []
    for name in source['patches']:
        if PurePosixPath(name).name != name:
            raise ValueError('unsafe patch path')
        path = HERE / 'patches' / name
        patch = path.read_bytes()
        touched = re.findall(rb'(?m)^\+\+\+ b/([^\r\n]+)', patch)
        for old, new in re.findall(rb'(?m)^diff --git a/([^\r\n ]+) b/([^\r\n ]+)$', patch):
            if old != new:
                raise ValueError('runtime patch renames a source file')
            touched.append(new)
        if not touched or any(p.decode() not in files for p in touched):
            raise ValueError('patch outside source inventory')
        patches.append({'path': 'tools/runtime-bundle/patches/' + name, 'sha256': digest(patch)})
    inputs = source.get('inputs', [])
    for item in inputs:
        path = (ROOT / item['path']).resolve()
        if not path.is_relative_to(ROOT) or not path.is_file() or digest(path.read_bytes()) != item['sha256']:
            raise ValueError('derived grammar input identity differs')
    output.mkdir(parents=True)
    for name, data in sorted(files.items()):
        if name.endswith('.go'):
            data = IMPORTS.sub(lambda m: m.group().replace(upstream.encode(), source['internal_module'].encode()), data)
        path = output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    for patch in patches:
        command = ['git', 'apply', '--directory=' + relative, str(ROOT / patch['path'])]
        subprocess.run(command[:2] + ['--check'] + command[2:], cwd=ROOT, check=True, timeout=30)
        subprocess.run(command, cwd=ROOT, check=True, timeout=30)
    main, optional = separate({name: (output / name).read_bytes() for name in files}, source['internal_module'])
    for name in files:
        if name not in main:
            (output / name).unlink()  # Only files just created in the new output.
        elif main[name] != (output / name).read_bytes():
            (output / name).write_bytes(main[name])
    if optional_output is not None:
        optional_output.mkdir(parents=True)
        for name, data in optional.items():
            path = optional_output / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        subprocess.run(['gofmt', '-w', *map(str, optional_output.glob('*.go'))], check=True, timeout=30)
    return {'schema': 1, 'baseline': source['baseline'],
            'archive_sha256': source['archive_sha256'],
            'source_sha256': digest((HERE / 'source.json').read_bytes()),
            'importer_sha256': digest(Path(__file__).read_bytes()),
            'separator_sha256': digest((HERE / 'separate.py').read_bytes()),
            'internal_module': source['internal_module'], 'patches': patches, 'inputs': inputs,
            'separated_files': {name: digest(data) for name, data in sorted(files.items()) if name not in main},
            'files': {name: {'origin_sha256': digest(data),
                             'sha256': digest((output / name).read_bytes())}
                      for name, data in sorted(files.items()) if name in main}}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--optional-output', type=Path)
    args = parser.parse_args()
    manifest = args.manifest.resolve()
    manifest.relative_to(ROOT)
    if manifest.exists():
        raise ValueError('manifest already exists')
    result = build(args.archive, args.output, args.optional_output)
    with manifest.open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
    print('bundled', len(result['files']), 'files; patches:', len(result['patches']))
