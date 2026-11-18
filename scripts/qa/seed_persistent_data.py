"""Append reusable fixtures to a stopped test database; preserve existing data."""

from datetime import date, datetime, time, timedelta, timezone
import json
from pathlib import Path
import sqlite3
import sys

path = Path(sys.argv[1]).resolve(strict=True)
today = date.fromisoformat(sys.argv[2])
backup_path = Path(sys.argv[3])
zone = timezone(timedelta(hours=8))
now = datetime.now(zone)
prefix = "测试 · "


def stamp(day, minute=480):
    return datetime.combine(day, time(), zone) + timedelta(minutes=minute)


def parse(value):
    dt = datetime.fromisoformat(value)
    return dt if dt.tzinfo else dt.replace(tzinfo=zone)


with sqlite3.connect(f"file:{path}?mode=rw", uri=True) as db:
    db.execute("PRAGMA foreign_keys=ON")
    if db.execute("SELECT 1 FROM subjects WHERE name=?", (prefix + "高等数学",)).fetchone():
        print("Persistent fixtures already exist; nothing changed.")
        raise SystemExit(0)
    if db.execute("SELECT 1 FROM sessions WHERE deleted=0 AND end_at IS NULL").fetchone():
        raise SystemExit("Stop the active study session before seeding.")
    backup_path.parent.mkdir(parents=True, exist_ok=True)
    if backup_path.exists():
        raise SystemExit("Backup already exists; refusing to replace it.")
    with sqlite3.connect(backup_path) as backup:
        db.backup(backup)

    names = ["高等数学", "大学英语", "数据结构", "计算机网络", "操作系统", "线性代数"]
    colors = ["#007AFF", "#34C759", "#FF9500", "#AF52DE", "#5AC8FA", "#5856D6"]
    intervals = [(parse(s), parse(e)) for s, e in db.execute(
        "SELECT start_at,end_at FROM sessions WHERE deleted=0 AND end_at IS NOT NULL"
    )]
    plan_windows = []
    for clock, duration in db.execute("SELECT start_time,duration_minutes FROM plans WHERE deleted=0 AND active=1"):
        hour, minute = map(int, clock.split(":"))
        plan_windows.append((hour * 60 + minute, hour * 60 + minute + duration))

    def free_plan(start, duration):
        return all(start + duration <= s or start >= e for s, e in plan_windows)

    def add_plan(subject, minute, duration, rest, color, note):
        plan_id = db.execute(
            "INSERT INTO plans(subject_id,start_time,duration_minutes,weekdays,active,color,break_minutes,note) VALUES(?,?,?,'0,1,2,3,4,5,6',1,?,?,?)",
            (subject, f"{minute // 60:02d}:{minute % 60:02d}", duration, color, rest, note),
        ).lastrowid
        plan_windows.append((minute, minute + duration))
        return plan_id

    links = []
    for index, (name, color) in enumerate(zip(names, colors)):
        subject = db.execute(
            "INSERT INTO subjects(name,color,created_at) VALUES(?,?,?)",
            (prefix + name, color, stamp(today - timedelta(days=30)).isoformat()),
        ).lastrowid
        minute = (8 + index) * 60
        while minute + 50 < 1440 and not free_plan(minute, 50):
            minute += 10
        if minute + 50 >= 1440:
            raise RuntimeError("No non-conflicting test plan slot; rolling back.")
        plan = add_plan(subject, minute, 50, 5, color, "固定测试数据：知识点复习、练习与错题整理")
        links.append((subject, plan, name, minute))

    # A current slot makes start/pause/resume available when the test app opens.
    current_plan = None
    current_minute = max(0, now.hour * 60 + now.minute - 5)
    if now.date() == today and current_minute + 45 < 1440 and free_plan(current_minute, 45):
        current_plan = add_plan(links[0][0], current_minute, 45, 5, colors[0], "固定测试数据：用于开始、暂停、继续和倒计时验证")

    def add_session(subject, plan, day, minute, seconds, source):
        start = stamp(day, minute)
        end = start + timedelta(seconds=seconds)
        if any(start < existing_end and end > existing_start for existing_start, existing_end in intervals):
            return False
        db.execute(
            "INSERT INTO sessions(subject_id,plan_id,start_at,planned_end_at,end_at,duration_seconds,source,note) VALUES(?,?,?,?,?,?,?,?)",
            (subject, plan if source == "timer" else None, start.isoformat(),
             (stamp(day, minute + 50).isoformat() if source == "timer" else None),
             end.isoformat(), seconds, source, "固定测试数据：章节练习与复习"),
        )
        intervals.append((start, end))
        return True

    session_count = 0
    for offset in range(14, 0, -1):
        day = today - timedelta(days=offset)
        targets = []
        actual_total = 0
        review_items = []
        for index, (subject, plan, name, minute) in enumerate(links):
            seconds = (18 + ((offset + index) % 6) * 6) * 60
            source = "manual" if (offset + index) % 3 == 0 else "timer"
            # Skip occupied windows rather than change or overlap existing records.
            added = add_session(subject, plan, day, minute, seconds, source)
            actual = seconds if added else 0
            session_count += int(added)
            actual_total += actual
            targets.append({"subjectId": subject, "subjectName": prefix + name, "plannedSeconds": 2700, "actualSeconds": 0, "reason": ""})
            review_items.append((day.isoformat(), subject, prefix + name, 2700, actual, "固定测试数据：复习与练习耗时不同，继续调整节奏。"))
        frozen = db.execute("SELECT targets_json FROM daily_plan_targets WHERE day=?", (day.isoformat(),)).fetchone()
        original_targets = json.loads(frozen[0]) if frozen else []
        existing_ids = {item["subjectId"] for item in original_targets}
        original_targets.extend(item for item in targets if item["subjectId"] not in existing_ids)
        db.execute("INSERT INTO daily_plan_targets(day,targets_json) VALUES(?,?) ON CONFLICT(day) DO UPDATE SET targets_json=excluded.targets_json", (day.isoformat(), json.dumps(original_targets, ensure_ascii=False)))
        # Existing submitted reviews are left intact.
        if not db.execute("SELECT 1 FROM daily_reviews WHERE day=?", (day.isoformat(),)).fetchone():
            actuals = dict(db.execute("SELECT subject_id,SUM(duration_seconds) FROM sessions WHERE deleted=0 AND end_at IS NOT NULL AND date(start_at,'localtime')=? GROUP BY subject_id", (day.isoformat(),)))
            db.execute("INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES(?,'project',80,?,?,?)", (day.isoformat(), sum(t["plannedSeconds"] for t in original_targets), sum(actuals.values()), stamp(day + timedelta(days=1), 450).isoformat()))
            db.executemany("INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES(?,?,?,?,?,?)", review_items)
        db.execute("INSERT INTO activity_logs(happened_at,content) VALUES(?,?)", (stamp(day, 1080).isoformat(), prefix + names[offset % len(names)] + "：完成复习、整理错题与学习小结。"))

    # Today's entries only occupy elapsed time and never overlap current study.
    elapsed = now.hour * 60 + now.minute if now.date() == today else 0
    ceiling = min(elapsed, current_minute) if current_plan else elapsed
    for index, minute in enumerate(range(0, min(ceiling, 60) - 9, 10)):
        subject, plan, _, _ = links[index % len(links)]
        session_count += int(add_session(subject, plan, today, minute, 600, "manual" if index % 2 == 0 else "timer"))

    goal_specs = [
        ("正常推进 · 数学专题复习", -2, 14, None),
        ("正常推进 · 英语词汇积累", -3, 21, None),
        ("正常推进 · 数据结构练习", -5, 30, None),
        ("时间过半 · 网络协议梳理", -20, 3, None),
        ("时间过半 · 操作系统复习", -20, 6, None),
        ("今天截止 · 线性代数测验", -14, 0, None),
        ("已经逾期 · 错题整理", -20, -2, None),
        ("已验收完成 · 第一轮复习", -30, -4, True),
        ("已验收未完成 · 阅读笔记", -30, -3, False),
    ]
    for title, created, due, completed in goal_specs:
        accepted = stamp(today - timedelta(days=1), 1080).isoformat() if completed is not None else None
        db.execute("INSERT INTO stage_goals(title,deadline,note,completed_at,accepted_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?)", (prefix + title, (today + timedelta(days=due)).isoformat(), "固定测试数据：用于查看正常、提醒、逾期和验收状态。", accepted if completed else None, accepted, stamp(today + timedelta(days=created)).isoformat(), now.isoformat()))
    if db.execute("PRAGMA foreign_key_check").fetchall():
        raise RuntimeError("Foreign-key check failed; rolling back.")
    if db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
        raise RuntimeError("Integrity check failed; rolling back.")
    print(json.dumps({"newSubjects": len(links), "newPlans": len(links) + int(current_plan is not None), "newSessions": session_count, "newGoals": len(goal_specs), "newActivities": 14, "integrity": "ok"}, ensure_ascii=False))
