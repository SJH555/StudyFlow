#!/usr/bin/env python3
"""准备独立的 Windows 启动与内存对照数据，不读取用户数据库。"""
import argparse
from datetime import datetime, timedelta, timezone
from pathlib import Path
import shutil
import sqlite3
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--before-binary', default='/tmp/studyflow-baseline/StudyFlow.exe')
parser.add_argument('--after-binary', default='/tmp/studyflow-tests/StudyFlow.exe')
parser.add_argument('--record-mode', choices=['manual', 'mixed', 'planned'], default='manual')
parser.add_argument('--history-days', type=int, default=365)
parser.add_argument('--temp-dir', default='/mnt/c/Users/jhsong/AppData/Local/Temp')
args = parser.parse_args()
if not 1 <= args.history_days <= 3650:
    parser.error('--history-days 必须在 1 到 3650 之间')
repo = Path(__file__).resolve().parents[2]
root = Path(tempfile.mkdtemp(prefix='StudyFlow-startup-memory-', dir=args.temp_dir))
(root / '.studyflow-qa').write_text('startup memory fixtures\n')
for variant, binary in [('before', args.before_binary), ('after', args.after_binary)]:
    target = root / variant
    (target / 'data').mkdir(parents=True)
    shutil.copyfile(binary, target / 'StudyFlow.exe')
    with sqlite3.connect(target / 'data/study.db') as db:
        db.executescript((repo / 'internal/infrastructure/upgrade/sql/00001_baseline.sql').read_text())
        db.execute('CREATE TABLE goose_db_version(id INTEGER PRIMARY KEY AUTOINCREMENT,version_id INTEGER NOT NULL,is_applied INTEGER NOT NULL,tstamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP)')
        db.executemany('INSERT INTO goose_db_version(version_id,is_applied) VALUES(?,1)', [(0,), (1,)])
        db.executemany('INSERT OR REPLACE INTO settings VALUES(?,?)', [('closeBehavior','quit'), ('autoBackupEnabled','1'), ('dailyReviewEnabled','0')])
        db.execute("INSERT INTO subjects(id,name,created_at) VALUES(1,'性能测试科目','2026-01-01T08:00:00+08:00')")
        today = datetime.now(timezone(timedelta(hours=8))).replace(hour=0, minute=0, second=0, microsecond=0)
        records = []
        for day in range(args.history_days):
            for index in range(24):
                started = today - timedelta(days=day) + timedelta(minutes=index*30)
                records.append((started.isoformat(), (started+timedelta(minutes=20)).isoformat(), '测试记录'*30))
        db.executemany("INSERT INTO sessions(subject_id,start_at,end_at,duration_seconds,source,note) VALUES(1,?,?,1200,'manual',?)", records)
        if args.record_mode != 'manual':
            db.execute("INSERT INTO plans(id,subject_id,start_time,duration_minutes,break_minutes) VALUES(1,1,'00:00',720,30)")
            condition = '1' if args.record_mode == 'planned' else 'id % 2 = 0'
            db.execute("UPDATE sessions SET plan_id=1,source='timer',break_used_seconds=1+id%3 WHERE " + condition)
(root / 'launch.ps1').write_text('''param([string]$Variant)
$ErrorActionPreference='Stop'
$dir=Join-Path $PSScriptRoot $Variant
$p=Start-Process -FilePath (Join-Path $dir 'StudyFlow.exe') -WorkingDirectory $dir -RedirectStandardOutput (Join-Path $dir 'out.txt') -RedirectStandardError (Join-Path $dir 'err.txt') -PassThru
$p.Id | Set-Content -LiteralPath (Join-Path $dir 'pid.txt') -Encoding ascii
Write-Output $p.Id
''')
(root / 'memory.ps1').write_text(r'''param([int]$RootPid)
$all=@(Get-CimInstance Win32_Process)
$ids=@($RootPid)
do {
 $next=@($all | Where-Object {$ids -contains $_.ParentProcessId -and $ids -notcontains $_.ProcessId} | ForEach-Object {$_.ProcessId})
 $ids += $next
} while ($next.Count -gt 0)
$processes=@(Get-Process -Id $ids -ErrorAction SilentlyContinue)
$details=@($processes | ForEach-Object {
 $item=$_
 $info=$all | Where-Object {$_.ProcessId -eq $item.Id} | Select-Object -First 1
 $role=if ($info.CommandLine -match '--type=(\S+)') {$Matches[1]} elseif ($item.ProcessName -eq 'StudyFlow') {'app'} else {'browser'}
 @{name=$item.ProcessName;role=$role;privateBytes=$item.PrivateMemorySize64;workingSet=$item.WorkingSet64}
})
@{processes=$details;count=$processes.Count;workingSet=($processes | Measure-Object WorkingSet64 -Sum).Sum;privateBytes=($processes | Measure-Object PrivateMemorySize64 -Sum).Sum} | ConvertTo-Json -Compress
''')
print(root)
