#!/usr/bin/env python3
"""Create real-SQL interruption fixtures; no production hooks or daily data."""
import contextlib
import io
import json
from pathlib import Path
import runpy
import shutil
import sqlite3

with contextlib.redirect_stdout(io.StringIO()):
    prepared = runpy.run_path(str(Path(__file__).with_name('prepare-upgrade-startup-windows.py')))
root, repo = prepared['root'], prepared['repo']
base = (repo / 'internal/infrastructure/upgrade/sql/00001_baseline.sql').read_text()
for journal in ['delete', 'wal']:
    for mode in ['close', 'exit', 'kill', 'repeat', 'committed', 'sqlerror', 'full']:
        fixture = root / 'cases' / f'{mode}-{journal}'
        (fixture / 'data').mkdir(parents=True)
        (fixture / 'sql').mkdir()
        (fixture / '.studyflow-upgrade-qa').write_text('temporary upgrade UI fixture\n')
        shutil.copyfile(root / 'cases/ready/data/study.db', fixture / 'data/study.db')
        with sqlite3.connect(fixture / 'data/study.db') as conn:
            conn.execute('PRAGMA journal_mode=' + journal)
            # Populate all nine business tables. Compare each after interruption
            # and resume, including settings and parent/child relationships.
            conn.execute("INSERT INTO plans(id,subject_id,start_time,duration_minutes) VALUES(1,1,'08:00',30)")
            conn.execute("INSERT INTO activity_logs VALUES(1,'2026-11-18T08:00:00+08:00','保留活动')")
            conn.execute("INSERT INTO daily_reviews VALUES('2026-11-17','manual',80,1800,180,'2026-11-18T08:00:00+08:00')")
            conn.execute("INSERT INTO daily_review_items VALUES(1,'2026-11-17',1,'升级验证科目',1800,180,'保留原因')")
            conn.execute("INSERT INTO daily_plan_targets VALUES('2026-11-17','{}')")
            conn.execute("INSERT INTO stage_goals VALUES(1,'保留目标','2026-12-01','保留备注',NULL,NULL,'2026-11-18T08:00:00+08:00','2026-11-18T08:00:00+08:00',0)")
            saved = {}
            for table in ['subjects','plans','sessions','activity_logs','settings','daily_reviews','daily_review_items','daily_plan_targets','stage_goals']:
                columns = [x[1] for x in conn.execute(f'PRAGMA table_info({table})')]
                rows = conn.execute(f'SELECT * FROM {table} ORDER BY rowid').fetchall()
                saved[table] = {'columns': columns, 'rows': rows}
        conn.close()
        (fixture / 'original-business.json').write_text(json.dumps(saved, ensure_ascii=False, indent=2))
        (fixture / 'sql/00001_baseline.sql').write_text(base)
        (fixture / 'sql/00002_probe.sql').write_text("-- +goose Up\nCREATE TABLE upgrade_probe(id INTEGER PRIMARY KEY,value TEXT); INSERT INTO upgrade_probe VALUES(1,'file2 committed');\n")
        body = """-- +goose Up
UPDATE sessions SET note='MUST ROLL BACK';
ALTER TABLE sessions ADD COLUMN qa_upgrade_column TEXT;
UPDATE upgrade_probe SET value='file3 incomplete';
CREATE TABLE upgrade_scratch(data BLOB);
INSERT INTO upgrade_scratch VALUES(zeroblob(8388608));
SELECT studyflow_upgrade_qa_checkpoint('in-sql');
"""
        if mode == 'sqlerror':
            body += "SELECT studyflow_upgrade_qa_checkpoint('fail-once');\n"
        else:
            body += "WITH RECURSIVE wait(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM wait WHERE n<100000 AND studyflow_upgrade_qa_checkpoint('tick')=0) SELECT sum(n) FROM wait;\n"
        body += "UPDATE sessions SET note='升级前保留的短学习记录'; UPDATE upgrade_probe SET value='file3 committed';\n"
        (fixture / 'sql/00003_probe.sql').write_text(body)
        if mode == 'committed':
            (fixture / 'release').touch()
# Separate extreme binary: target 3 and synthetic SQL exist only in Go overlay.
version = repo / 'internal/infrastructure/upgrade/version.go'
s = version.read_text().replace('"io/fs"', '"io/fs"\n "os"\n "path/filepath"\n "strings"')
s = s.replace('const TargetVersion int64 = 1', 'const TargetVersion int64 = 3')
a = s.index('func New() (*Manager, error) {')
s = s[:a] + '''func New() (*Manager, error) {
 exe, err := os.Executable(); if err != nil { return nil, err }
 dir := filepath.Dir(exe)
 selected, err := os.ReadFile(filepath.Join(dir,"case.txt")); if err != nil { return nil, err }
 name := strings.TrimSpace(string(selected))
 if !filepath.IsLocal(name) || strings.ContainsAny(name,"/\\\\") { return nil, errors.New("invalid QA case") }
 return newManager(os.DirFS(filepath.Join(dir,"cases",name,"sql")),TargetVersion)
}
'''
# Keep fs import needed by embedding declaration only in production New.
s = s.replace('"io/fs"', '')
(root / 'version-ui-overlay.go').write_text(s)
overlay = json.loads((root / 'overlay.json').read_text())
overlay['Replace'][str(version)] = str(root / 'version-ui-overlay.go')
(root / 'overlay-extreme.json').write_text(json.dumps(overlay))
launch = (root / 'launch-ui.ps1').read_text().replace('Write-Output "QA process=$($p.Id)"','Set-Content -LiteralPath (Join-Path $PSScriptRoot "cases\\$Case\\process-id.txt") -Value $p.Id\nWrite-Output "QA process=$($p.Id)"')
(root / 'launch-ui.ps1').write_text(launch)
print(root)
