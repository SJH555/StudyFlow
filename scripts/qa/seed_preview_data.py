"""应用完全退出后，在指定测试库追加有关联的展示数据；不清空已有记录。"""

from datetime import date, datetime, timedelta, timezone
import json
from pathlib import Path
import sqlite3
import sys


path = Path(sys.argv[1]).resolve(strict=True)
today = date.fromisoformat(sys.argv[2])
zone = timezone(timedelta(hours=8))


def stamp(day, hour=8, minute=0):
    return datetime.combine(day, datetime.min.time(), zone).replace(
        hour=hour, minute=minute
    ).isoformat()


with sqlite3.connect(path) as db:
    db.execute("PRAGMA foreign_keys=ON")
    if db.execute("SELECT 1 FROM subjects WHERE name='高等数学 · 展示'").fetchone():
        raise SystemExit("展示数据已存在，停止以避免重复追加")
    backup_path = Path(sys.argv[3])
    with sqlite3.connect(backup_path) as backup:
        db.backup(backup)
    subjects = ["高等数学", "大学英语", "数据结构", "计算机网络", "操作系统", "线性代数", "阅读与写作"]
    colors = ["#007AFF", "#34C759", "#FF9500", "#AF52DE", "#5AC8FA", "#FF3B30", "#5856D6"]
    reasons = ["难题耗时较长，明天先复习例题。", "单词复习完成，听力练习需要补足。", "调试链表代码占用了额外时间。", "协议概念仍需通过画图梳理。", "完成进程调度练习，继续保持。", "矩阵计算速度有提升空间。", "已完成阅读并整理笔记。"]
    links = []
    for index, (name, color) in enumerate(zip(subjects, colors)):
        subject_id = db.execute(
            "INSERT INTO subjects(name,color,created_at,sort_order) VALUES(?,?,?,?)",
            (name + " · 展示", color, stamp(today - timedelta(days=30)), index),
        ).lastrowid
        plan_id = db.execute(
            "INSERT INTO plans(subject_id,start_time,duration_minutes,weekdays,active,color,break_minutes,note) VALUES(?,?,35,'1,2,3,4,5,6,0',1,?,5,?)",
            (subject_id, f"{8 + index:02d}:00", color, f"{name}：知识点复习、例题练习与错题整理。"),
        ).lastrowid
        links.append((subject_id, plan_id, name + " · 展示"))

    # 连续七天历史，加今天的七条已结束记录；时长与总结、快照逐项对应。
    for day_index in range(8):
        day = today - timedelta(days=7 - day_index)
        targets = []
        total_actual = 0
        items = []
        for index, (subject_id, plan_id, name) in enumerate(links):
            seconds = 1080 + ((day_index + index) % 5) * 180
            start = datetime.fromisoformat(stamp(day, 8 + index))
            end = start + timedelta(seconds=seconds)
            db.execute(
                "INSERT INTO sessions(subject_id,plan_id,start_at,planned_end_at,end_at,duration_seconds,source,note) VALUES(?,?,?,?,?,?,'timer',?)",
                (subject_id, plan_id, start.isoformat(), (start + timedelta(minutes=35)).isoformat(), end.isoformat(), seconds, f"{name}：完成当天章节与练习，整理了易错点。"),
            )
            targets.append({"subjectId": subject_id, "subjectName": name, "plannedSeconds": 1800, "actualSeconds": 0, "reason": ""})
            items.append((day.isoformat(), subject_id, name, 1800, seconds, reasons[index] if seconds < 1440 else "已达到目标，继续保持。"))
            total_actual += seconds
        db.execute(
            "INSERT OR IGNORE INTO daily_plan_targets(day,targets_json) VALUES(?,?)",
            (day.isoformat(), json.dumps(targets, ensure_ascii=False)),
        )
        if day < today:
            if db.execute("SELECT 1 FROM daily_reviews WHERE day=?", (day.isoformat(),)).fetchone():
                raise RuntimeError(f"{day} 已有总结，回滚本次数据，避免覆盖")
            db.execute(
                "INSERT INTO daily_reviews(day,mode,threshold,planned_seconds,actual_seconds,submitted_at) VALUES(?,'project',80,?,?,?)",
                (day.isoformat(), 7 * 1800, total_actual, stamp(day + timedelta(days=1), 7, 30)),
            )
            db.executemany(
                "INSERT INTO daily_review_items(day,subject_id,subject_name,planned_seconds,actual_seconds,reason) VALUES(?,?,?,?,?,?)",
                items,
            )
        db.execute(
            "INSERT INTO activity_logs(happened_at,content) VALUES(?,?)",
            (stamp(day, 15, 30), f"学习回顾：整理{subjects[day_index % 7]}笔记，完成当天练习，并安排次日复习。"),
        )

    goals = [
        ("完成数学第一轮复习", 3, None, None),
        ("英语核心词汇复习", 7, None, None),
        ("完成数据结构专项练习", 14, None, None),
        ("网络协议知识图谱", 21, None, None),
        ("操作系统章节测验", 30, None, None),
        ("线性代数基础巩固", -2, -3, -2),
        ("完成阅读笔记周总结", -1, None, -1),
    ]
    for title, offset, completed, accepted in goals:
        db.execute(
            "INSERT INTO stage_goals(title,deadline,note,completed_at,accepted_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?)",
            (title, (today + timedelta(days=offset)).isoformat(), f"{title}：按每日学习计划推进，结合学习记录和每日总结验收。", stamp(today + timedelta(days=completed), 18) if completed is not None else None, stamp(today + timedelta(days=accepted), 18) if accepted is not None else None, stamp(today - timedelta(days=20)), stamp(today - timedelta(days=1), 18)),
        )
    # 使用真实配置键，保留已有设置，不制造无业务含义的占位键。
    defaults = {
        "dailyReviewEnabled": "0", "dailyReviewMode": "project",
        "dailyReviewThreshold": "80", "dailyReviewStartDate": today.isoformat(),
        "dailyReviewCheckHour": "6", "dailyReviewCheckMinute": "0",
        "ringtoneVolume": "100", "startupWindow": "show", "closeBehavior": "tray",
        "primaryNavWidth": "250", "sectionNavWidth": "240",
    }
    db.executemany("INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)", defaults.items())
    # 快照若已存在，合并新增项目，保留旧项目冻结的目标。
    for day_index in range(8):
        day = (today - timedelta(days=day_index)).isoformat()
        raw = db.execute("SELECT targets_json FROM daily_plan_targets WHERE day=?", (day,)).fetchone()[0]
        targets = json.loads(raw)
        existing_ids = {item["subjectId"] for item in targets}
        for subject_id, _, name in links:
            if subject_id not in existing_ids:
                targets.append({"subjectId": subject_id, "subjectName": name, "plannedSeconds": 1800, "actualSeconds": 0, "reason": ""})
        db.execute("UPDATE daily_plan_targets SET targets_json=? WHERE day=?", (json.dumps(targets, ensure_ascii=False), day))
    if db.execute("PRAGMA foreign_key_check").fetchall():
        raise RuntimeError("失效外键，回滚本次追加")
    if db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
        raise RuntimeError("完整性检查未通过")
    for table, in db.execute("SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'").fetchall():
        count = db.execute(f'SELECT count(*) FROM "{table}"').fetchone()[0]
        if count < 7:
            raise RuntimeError(f"{table} 少于七行")
        print(f"{table}: {count}")
    print("integrity_check=ok; foreign_key_check=empty")
