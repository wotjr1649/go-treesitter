"""Check committed official module ZIPs and offline empty-cache consumers."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
MODULE = 'github.com/wotjr1649/go-treesitter'
VERSION = 'v0.0.1'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def objects(data):
    decoder = json.JSONDecoder()
    while data.strip():
        value, end = decoder.raw_decode(data.lstrip())
        yield value
        data = data.lstrip()[end:]


def check_graph(data, optional):
    rows = list(objects(data))
    expected = {'example.com/release-consumer', MODULE}
    if optional:
        expected.add(MODULE + '/grammars/gpl')
    require(len(rows) == len(expected) and {r['Path'] for r in rows} == expected,
            'unexpected consumer module graph')
    require(not any('Replace' in r for r in rows), 'consumer uses replace')
    require(all(r.get('Version') == VERSION for r in rows if r['Path'] != 'example.com/release-consumer'),
            'consumer resolved a different version')
    return [{k: r[k] for k in ('Path', 'Version', 'Main') if k in r} for r in rows]


def check_archive(archive, module, optional):
    prefix = module + '@' + VERSION + '/'
    paths = archive.namelist()
    require(all(p.startswith(prefix) for p in paths), 'invalid module ZIP prefix')
    names = [p.removeprefix(prefix) for p in paths]
    require(len(names) == len(set(names)), 'duplicate module entries')
    require({'LICENSE', 'go.mod'} <= set(names), 'missing license or module')
    require(not any(n.startswith(('artifacts/', '.scratch/', 'experiments/', 'docs/prompts/',
                                  'docs/plans/', 'tools/modulezip/')) for n in names),
            'local evidence or nested tooling entered the module ZIP')
    if optional:
        require(sum(n.endswith('.bin') for n in names) == 3, 'GPL blob count changed')
        require(any(n.startswith('sources/') and n.endswith('.zip') for n in names), 'missing GPL sources')
        required = ['provenance.json', 'sources/ts2go-source.json', 'LICENSE']
        subdir = 'grammars/gpl/'
    else:
        require(not any(n.startswith('grammars/gpl/') for n in names), 'GPL module leaked into base ZIP')
        require(sum(n.startswith('internal/runtime/grammars/grammar_blobs/') and n.endswith('.bin')
                    for n in names) == 203, 'base grammar count changed')
        required = ['LICENSES/catalog.json', 'tools/runtime-bundle/grammars/go-source.zip',
                    'tools/runtime-bundle/grammars/go.json', 'internal/provenance/runtime.json']
        subdir = ''
    for name in required:
        require(archive.read(prefix + name) == (ROOT / (subdir + name)).read_bytes(),
                'packaged provenance differs: ' + name)
    return names, archive.read(prefix + 'go.mod')


def main():
    require(len(sys.argv) == 2, 'usage: python tools/release/check.py PATH_TO_MODULEZIP')
    helper = Path(sys.argv[1]).resolve()
    scratch = ROOT / '.scratch'
    scratch.mkdir(exist_ok=True)
    directory = Path(tempfile.mkdtemp(prefix='release-check-', dir=scratch))
    env = {k: v for k, v in os.environ.items() if k.upper() in
           ('SYSTEMROOT', 'WINDIR', 'COMSPEC', 'PATHEXT', 'PATH')}
    env.update(CGO_ENABLED='0', GOENV='off', GOFLAGS='', GOWORK='off', GOTOOLCHAIN='local',
               GOPROXY='off', GOSUMDB='off', GONOPROXY='none', GOPRIVATE='', GONOSUMDB='',
               GOCACHE=str(scratch / 'release-build-cache'), GOMODCACHE=str(directory / 'unused-cache'),
               TEMP=str(directory), TMP=str(directory), GOMAXPROCS='4')
    executions = []

    def run(label, command, cwd=ROOT, current=env, timeout=180):
        log = directory / (label + '.log')
        with log.open('xb') as stream:
            process = subprocess.Popen(command, cwd=cwd, env=current, stdout=stream, stderr=subprocess.STDOUT)
            try:
                code = process.wait(timeout=timeout)
            except subprocess.TimeoutExpired:
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               env=env, stdout=stream, stderr=subprocess.STDOUT, timeout=15, check=True)
                process.wait(timeout=15)
                raise
        data = log.read_bytes()
        executions.append({'check': label, 'exit': code, 'log_sha256': sha(data)})
        print(label, 'exit', code, flush=True)
        if code:
            print(data.decode('utf-8', errors='replace'), flush=True)
            raise RuntimeError(label + ' failed; original local log retained')
        return data.decode('utf-8')

    head = run('commit', ['git', 'rev-parse', 'HEAD']).strip()
    require(re.fullmatch(r'[0-9a-f]{40}', head), 'expected an exact candidate commit')
    require(not run('status', ['git', 'status', '--porcelain']).strip(), 'candidate worktree is not clean')
    platform = json.loads(run('go-env', ['go', 'env', '-json', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'GOVERSION']))
    require(platform['GOOS'] == 'windows' and platform['CGO_ENABLED'] == '0', 'Windows product lane required')
    modules = []
    for optional in (False, True):
        label = 'gpl' if optional else 'main'
        subdir = 'grammars/gpl' if optional else ''
        module = MODULE + ('/grammars/gpl' if optional else '')
        output = directory / (label + '.zip')
        run('zip-' + label, [str(helper), module, str(ROOT), head, subdir, str(output)])
        with zipfile.ZipFile(output) as archive:
            names, mod = check_archive(archive, module, optional)
        proxy = directory / 'proxy' / module / '@v'
        proxy.mkdir(parents=True)
        shutil.copyfile(output, proxy / (VERSION + '.zip'))
        (proxy / (VERSION + '.mod')).write_bytes(mod)
        (proxy / (VERSION + '.info')).write_text(json.dumps({'Version': VERSION, 'Time': '2026-09-23T00:00:00Z'}), encoding='utf-8')
        modules.append({'module': module, 'version': VERSION, 'sha256': sha(output.read_bytes()),
                        'bytes': output.stat().st_size, 'entries': len(names)})
    consumers = []
    catalog_exes = []
    for optional in (False, True):
        label = 'gpl' if optional else 'main'
        consumer = directory / ('consumer-' + label)
        consumer.mkdir()
        mod = f'module example.com/release-consumer\n\ngo 1.27.1\n\nrequire {MODULE} {VERSION}\n'
        if optional:
            mod += f'require {MODULE}/grammars/gpl {VERSION}\n'
        (consumer / 'go.mod').write_text(mod, encoding='utf-8', newline='\n')
        shutil.copyfile(ROOT / ('testdata/consumer-gpl/main.go' if optional else 'testdata/consumer/main.go'), consumer / 'main.go')
        current = dict(env, GOMODCACHE=str(consumer / 'cache'), GOPROXY=(directory / 'proxy').as_uri())
        require(not (consumer / 'cache').exists(), 'consumer module cache is not empty')
        exe = consumer / 'consumer.exe'
        run('build-' + label, ['go', 'build', '-mod=mod', '-trimpath', '-o', str(exe), '.'], consumer, current)
        run('execute-' + label, [str(exe)], consumer, current, timeout=60)
        graph = check_graph(run('graph-' + label, ['go', 'list', '-m', '-json', 'all'], consumer, current), optional)
        deps = list(objects(run('deps-' + label, ['go', 'list', '-deps', '-json', '.'], consumer, current)))
        require(not any(r.get('CgoFiles') or r['ImportPath'] == 'runtime/cgo' for r in deps), 'CGO entered product graph')
        consumers.append({'module': label, 'graph': graph, 'executable_sha256': sha(exe.read_bytes()),
                          'dependency_packages': len(deps), 'cgo_files': 0, 'empty_module_cache': True})
        catalog = consumer / 'catalog'
        catalog.mkdir()
        shutil.copyfile(ROOT / 'tools/catalog/main.go', catalog / 'main.go')
        if optional:
            (catalog / 'gpl.go').write_text(f'package main\nimport _ "{MODULE}/grammars/gpl"\n', encoding='utf-8')
        catalog_exe = consumer / 'catalog.exe'
        run('catalog-build-' + label, ['go', 'build', '-mod=readonly', '-trimpath', '-o', str(catalog_exe), './catalog'], consumer, current)
        catalog_exes.append(catalog_exe)
    catalog_output = directory / 'catalog.json'
    run('catalog-206', [sys.executable, 'tools/catalog/check.py', '--executable', str(catalog_exes[0]),
                       '--gpl-executable', str(catalog_exes[1]), '--output', str(catalog_output)])
    catalog = json.loads(catalog_output.read_text(encoding='utf-8'))
    require(len(catalog['rows']) == 206 and all(r['exit'] == 0 for r in catalog['rows']), 'incomplete catalog gate')
    result = {'status': 'PASS', 'candidate_commit': head, 'environment': platform,
              'modulezip_sha256': sha(helper.read_bytes()), 'modules': modules,
              'consumers': consumers, 'executions': executions,
              'catalog_basic': {'passed': 206, 'receipt_sha256': sha(catalog_output.read_bytes()),
                                'cases_sha256': catalog['cases_sha256']},
              'network': 'task-local file proxy only; no replace directives'}
    output = ROOT / 'artifacts/release-ci/packaging.json'
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open('x', encoding='utf-8', newline='\n') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
    print('official module ZIPs and empty-cache native consumers PASS')


if __name__ == '__main__':
    main()
