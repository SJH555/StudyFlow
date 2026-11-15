"""在 WSL 生成便携版 Windows 启动测试的隔离数据，不访问真实用户库。"""

import hashlib
import json
from pathlib import Path
import re
import shutil
import sqlite3
import sys


root = Path(sys.argv[1]).resolve()
source = Path(__file__).resolve().parents[2]
exe = root.parent / "StudyFlow-0.4.0-71-portable" / "StudyFlow.exe"
baseline = root / "切换目标 #100%" / "study.db"
text = (source / "desktop/startup_database_test.go").read_text()
legacy_function = text.split("func seedHistoricalStartupDatabase", 1)[1]
base_ddl = re.findall(r"`([^`]+)`", legacy_function.split("if version >= 2", 1)[0])
manifest = []
for name in ["existing", "legacy1", "legacy2", "legacy3", "corrupt", "empty", "unknown", "missing", "incomplete"]:
    folder = root / ("package-" + name)
    folder.mkdir(exist_ok=False)
    shutil.copy2(exe, folder / "StudyFlow.exe")
    data = folder / "data"
    data.mkdir()
    db_path = data / "study.db"
    if name == "existing" or name == "incomplete":
        shutil.copy2(baseline, db_path)
        with sqlite3.connect(db_path) as db:
            db.execute("INSERT INTO subjects(name,created_at) VALUES('package-marker','2026-11-15')")
            db.execute("UPDATE settings SET value='0' WHERE key='autoBackupEnabled'")
            if name == "incomplete":
                db.execute("ALTER TABLE plans DROP COLUMN note")
    elif name.startswith("legacy"):
        version = int(name[-1])
        with sqlite3.connect(db_path) as db:
            for statement in base_ddl:
                db.execute(statement)
            if version >= 2:
                db.execute("ALTER TABLE subjects ADD COLUMN archived INTEGER NOT NULL DEFAULT 0")
            if version == 3:
                for statement in re.findall(r"`([^`]+)`", legacy_function.split("if version == 3", 1)[1].split("seedStartupSQLite", 1)[0]):
                    db.execute(statement)
    elif name == "corrupt":
        db_path.write_bytes(b"qa: not a SQLite database")
    elif name == "empty":
        db_path.write_bytes(b"")
    elif name == "unknown":
        with sqlite3.connect(db_path) as db:
            db.execute("CREATE TABLE unrelated(id INTEGER)")
    elif name == "missing":
        windows_data = "D:\\CLink\\Desktop\\studyflow\\db-qa-20261115\\package-missing\\data"
        (folder / "storage-location.txt").write_text(windows_data + "\n")
    manifest.append({"name": name, "before_hash": hashlib.sha256(db_path.read_bytes()).hexdigest() if db_path.exists() else None})
(root / "package-manifest.json").write_text(json.dumps(manifest, indent=2))
print("Prepared", len(manifest), "isolated package fixtures")
