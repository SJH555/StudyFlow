#!/usr/bin/env python3
"""Check the committed Goose file manifest without network or database access."""
import hashlib
import json
from pathlib import Path

repo = Path(__file__).resolve().parents[2]
manifest = json.loads((repo / 'third_party/goose-upstream.json').read_text())
root = repo / 'third_party/goose'
expected = manifest['files']
for name, info in expected.items():
    file = root / name
    digest = hashlib.sha256(file.read_bytes()).hexdigest()
    if digest != info['local_sha256']:
        raise SystemExit(f'Goose source drift: {name}; review and update provenance/patch intentionally')
actual = {str(p.relative_to(root)) for p in root.rglob('*') if p.is_file()}
extra = actual - set(expected) - {'STUDYFLOW.md'}
if extra:
    raise SystemExit(f'Unrecorded Goose files: {sorted(extra)}')
patched = sorted(name for name, info in expected.items() if info['local_sha256'] != info['upstream_sha256'])
if patched != ['provider_options.go', 'provider_run.go']:
    raise SystemExit(f'Unexpected Goose patch scope: {patched}')
if 'replace github.com/pressly/goose/v3 => ./third_party/goose' not in (repo / 'go.mod').read_text():
    raise SystemExit('StudyFlow transaction adaptation requires the local Goose replace')
print(json.dumps({'passed': True, 'module': manifest['module'], 'version': manifest['version'], 'verifiedFiles': len(expected), 'patchedFiles': patched}, indent=2))
