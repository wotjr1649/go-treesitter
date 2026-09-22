"""Resolve the current generator or identify checked-in C, without version pins."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess


def identity(runtime_header, parser, generator_version=None):
    header = runtime_header.read_text(encoding='utf-8')
    source = parser.read_bytes()
    minimum = int(re.search(r'#define TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION (\d+)', header)[1])
    maximum = int(re.search(r'#define TREE_SITTER_LANGUAGE_VERSION (\d+)', header)[1])
    abi = int(re.search(rb'#define LANGUAGE_VERSION (\d+)', source)[1])
    if not minimum <= abi <= maximum:
        raise ValueError(f'ABI {abi} outside runtime range {minimum}..{maximum}')
    return {'generator_mode': 'generated' if generator_version else 'checked-in',
            'generator_version': generator_version, 'abi': abi,
            'runtime_abi_min': minimum, 'runtime_abi_max': maximum,
            'runtime_header_sha256': hashlib.sha256(runtime_header.read_bytes()).hexdigest(),
            'parser_sha256': hashlib.sha256(source).hexdigest()}


def generate(runtime_header, parser):
    """Resolve the installed CLI each time; use inert grammar JSON when present."""
    parser = parser.resolve()
    if not parser.is_relative_to(Path(__file__).resolve().parents[2]):
        raise ValueError('generation must stay inside this repository')
    command = shutil.which(os.environ.get('TREE_SITTER_CLI', 'tree-sitter'))
    if not command:
        raise ValueError('tree-sitter generator unavailable')
    version = subprocess.check_output([command, '--version'], text=True, timeout=10).strip()
    if not version:
        raise ValueError('generator returned no version identity')
    header = runtime_header.read_text(encoding='utf-8')
    abi = re.search(r'#define TREE_SITTER_LANGUAGE_VERSION (\d+)', header)[1]
    grammar = parser.with_name('grammar.json')
    if not grammar.is_file():
        raise ValueError('generation requires checked grammar.json; JavaScript is not executed')
    subprocess.run([command, 'generate', '--abi', abi, str(grammar)],
                   cwd=parser.parent.parent, check=True, timeout=120)
    result = identity(runtime_header, parser, version)
    result['generator_executable_sha256'] = hashlib.sha256(Path(command).read_bytes()).hexdigest()
    return result


if __name__ == '__main__':
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument('--runtime-header', required=True, type=Path)
    args.add_argument('--parser', required=True, type=Path)
    args.add_argument('--generate', action='store_true', help='generate in the parser parent grammar directory')
    options = args.parse_args()
    if options.generate:
        result = generate(options.runtime_header, options.parser)
    else:
        result = identity(options.runtime_header, options.parser)
    print(json.dumps(result, indent=2))
