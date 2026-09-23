"""Report unresolved upstream license declarations before a release."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
catalog = json.loads((ROOT / 'LICENSES/catalog.json').read_text(encoding='utf-8'))
rows = catalog['rows']
if len(rows) != 206 or len({r['grammar'] for r in rows}) != 206:
    raise ValueError('the complete license inventory is required')
blocked = [{'grammar': row['grammar'], 'commit': row['commit'],
            'declarations': row['declared_licenses'], 'source': row['source'],
            'additional_evidence': row['additional_evidence']}
           for row in rows if row.get('review_status') == 'requires_upstream_clarification']
print(json.dumps({'status': 'BLOCKED_EXTERNAL' if blocked else 'PASS',
                  'grammars': len(rows), 'unresolved_declarations': blocked}, indent=2))
raise SystemExit(bool(blocked))
