#!/usr/bin/env python3
"""Read-only checks after the disposable Windows WebView startup scenarios."""
import hashlib
import json
from pathlib import Path
import sqlite3
import sys

root = Path(sys.argv[1])
before = json.loads((root / 'original-hashes.json').read_text())
report = []
for name in ['baseline', 'cancel', 'backupfail', 'newer', 'unknown', 'fresh', 'ready']:
    fixture = root / 'cases' / name
    assert (fixture / '.studyflow-upgrade-qa').read_text() == 'temporary upgrade UI fixture\n'
    db = fixture / 'data/study.db'
    digest = hashlib.sha256(db.read_bytes()).hexdigest()
    conn = sqlite3.connect(f'file:{db}?mode=ro', uri=True)
    versions = conn.execute('SELECT version_id FROM goose_db_version ORDER BY id').fetchall() if conn.execute("SELECT COUNT(*) FROM sqlite_schema WHERE name='goose_db_version'").fetchone()[0] else []
    if name in ['baseline', 'fresh', 'ready']:
        assert versions == [(0,), (1,)], (name, versions)
    if name in ['cancel', 'backupfail', 'newer', 'unknown']:
        assert digest == before[name], f'{name} file changed'
    if name in ['baseline', 'cancel', 'backupfail', 'newer', 'ready']:
        assert conn.execute('SELECT duration_seconds,source,note FROM sessions').fetchall() == [(180, 'manual', '升级前保留的短学习记录')]
    copies = list((db.parent / 'backups').glob('upgrade-*.db')) if (db.parent / 'backups').is_dir() else []
    if name == 'baseline':
        assert len(copies) == 1
        backup = sqlite3.connect(f'file:{copies[0]}?mode=ro', uri=True)
        assert backup.execute("SELECT count(*) FROM sqlite_schema WHERE name='goose_db_version'").fetchone()[0] == 0
        assert backup.execute('SELECT duration_seconds,source,note FROM sessions').fetchall() == conn.execute('SELECT duration_seconds,source,note FROM sessions').fetchall()
        assert backup.execute('PRAGMA quick_check').fetchone()[0] == 'ok'
        backup.close()
    if name in ['fresh', 'ready']:
        assert not copies
    conn.close()
    report.append({'case': name, 'passed': True, 'unchangedBytes': digest == before.get(name), 'versions': [v[0] for v in versions], 'retainedBackups': len(copies)})
print(json.dumps(report, indent=2))
