#!/usr/bin/env python3
"""Prepare marker-owned, disposable Windows startup UI fixtures from WSL."""
import hashlib
import json
from pathlib import Path
import shutil
import sqlite3
import tempfile

repo = Path(__file__).resolve().parents[2]
root = Path(tempfile.mkdtemp(prefix='StudyFlow-upgrade-stage2-', dir='/mnt/c/Users/jhsong/AppData/Local/Temp'))
sql = (repo / 'internal/infrastructure/upgrade/sql/00001_baseline.sql').read_text()
hashes = {}
for case in ['baseline', 'cancel', 'backupfail', 'newer', 'unknown', 'fresh', 'ready']:
    fixture = root / 'cases' / case
    (fixture / 'data').mkdir(parents=True)
    (fixture / '.studyflow-upgrade-qa').write_text('temporary upgrade UI fixture\n')
    if case == 'fresh':
        continue
    db = fixture / 'data/study.db'
    with sqlite3.connect(db) as conn:
        if case == 'unknown':
            conn.execute('CREATE TABLE unrelated(id INTEGER)')
        else:
            conn.executescript(sql)
            conn.execute("INSERT OR REPLACE INTO settings VALUES('closeBehavior','quit')")
            conn.execute("INSERT INTO subjects(id,name,created_at) VALUES(1,'升级验证科目','2026-11-18T08:00:00+08:00')")
            conn.execute("INSERT INTO sessions(id,subject_id,start_at,end_at,duration_seconds,source,note) VALUES(1,1,'2026-11-18T08:00:00+08:00','2026-11-18T08:03:00+08:00',180,'manual','升级前保留的短学习记录')")
            if case in ['newer', 'ready']:
                conn.execute('CREATE TABLE goose_db_version(id INTEGER PRIMARY KEY AUTOINCREMENT,version_id INTEGER NOT NULL,is_applied INTEGER NOT NULL,tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP)')
                for version in range(3 if case == 'newer' else 2):
                    conn.execute('INSERT INTO goose_db_version(version_id,is_applied) VALUES(?,1)', (version,))
    conn.close()
    hashes[case] = hashlib.sha256(db.read_bytes()).hexdigest()
    if case == 'backupfail':
        (fixture / 'data/backups').write_text('occupied for upgrade failure QA')
(root / 'original-hashes.json').write_text(json.dumps(hashes, indent=2))
shutil.copyfile(repo / 'docs/database-upgrade-stage1-2026-11-18/windows-run-tests.ps1', root / 'windows-run-tests.ps1')
# Wails clears WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS. Enable CDP in this
# disposable compiler overlay only; the shipped application has no QA hook.
source = repo / 'desktop/main.go'
main = source.read_text().replace('application.WindowsOptions{DisableQuitOnLastWindowClosed: true}', 'application.WindowsOptions{DisableQuitOnLastWindowClosed: true, AdditionalBrowserArgs: []string{"--remote-debugging-port=19348"}}')
assert main != source.read_text()
(root / 'main-ui-overlay.go').write_text(main)
(root / 'overlay.json').write_text(json.dumps({'Replace': {str(source): str(root / 'main-ui-overlay.go')}}))
(root / 'launch-ui.ps1').write_text('''param([string]$Case)
$ErrorActionPreference='Stop'
Set-Location -LiteralPath $PSScriptRoot
Set-Content -LiteralPath (Join-Path $PSScriptRoot 'case.txt') -Value $Case -Encoding ascii
$env:Path="$env:SystemRoot\\System32;$env:Path"
$env:PATHEXT='.COM;.EXE;.BAT;.CMD'
$p=Start-Process -FilePath (Join-Path $PSScriptRoot 'upgrade-ui.exe') -WorkingDirectory $PSScriptRoot -RedirectStandardOutput (Join-Path $PSScriptRoot "$Case-ui-out.txt") -RedirectStandardError (Join-Path $PSScriptRoot "$Case-ui-err.txt") -PassThru
Write-Output "QA process=$($p.Id)"
''')
print(root)
