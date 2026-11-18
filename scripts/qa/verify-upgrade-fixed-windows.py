#!/usr/bin/env python3
"""Read-only fixed-directory acceptance AFTER confirming the app has exited."""
import hashlib
import json
from pathlib import Path
import sqlite3

root = Path('/mnt/d/CLink/Desktop/studyflow/StudyFlow-0.5.0-5-portable')
out = Path('docs/database-upgrade-stage3-2026-11-18')
expected = json.loads((root / 'qa-backups/before-goose-stage3-business.json').read_text())
before = json.loads((out / 'before-upgrade-summary.json').read_text())

def verify(path, tracked):
    conn = sqlite3.connect(f'file:{path}?mode=ro', uri=True)
    assert conn.execute('PRAGMA integrity_check').fetchall() == [('ok',)]
    assert not conn.execute('PRAGMA foreign_key_check').fetchall()
    for table, saved in expected.items():
        cols = ','.join('"' + col + '"' for col in saved['columns'])
        rows = [list(row) for row in conn.execute(f'SELECT {cols} FROM "{table}" ORDER BY rowid')]
        assert rows == saved['rows'], f'{table} changed'
    ledger = conn.execute("SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'").fetchone()[0]
    if tracked:
        assert conn.execute('SELECT version_id,is_applied FROM goose_db_version ORDER BY id').fetchall() == [(0, 1), (1, 1)]
    else:
        assert ledger == 0
    conn.close()

verify(root / 'data/study.db', True)
backups = list((root / 'data/backups').glob('upgrade-*.db'))
assert len(backups) == 1, 'same-version startup must not create another upgrade backup'
verify(backups[0], False)
for file, digest in before['userFilesSha256'].items():
    assert hashlib.sha256((root / 'data' / file).read_bytes()).hexdigest() == digest
result = {'passed': True, 'allNineTablesUnchanged': True, 'originalRecordCounts': {k: len(v['rows']) for k, v in expected.items()}, 'versions': [0, 1], 'verifiedPreUpgradeBackup': str(backups[0]), 'userFilesUnchanged': True, 'integrity': 'ok', 'foreignKeys': 'ok'}
(out / 'fixed-data-verification.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
print('PASS: fixed data, all nine original business tables, version ledger, backup and user files')
