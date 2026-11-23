-- +goose Up
-- 稳定 ID 标识整个项目或计划，行 ID 标识一次版本。
CREATE TABLE subject_versions (
    id INTEGER PRIMARY KEY,
    subject_id INTEGER NOT NULL,
    effective_day TEXT NOT NULL,
    name TEXT NOT NULL,
    color TEXT NOT NULL,
    sort_order INTEGER,
    created_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active','stopped','deleted'))
);
CREATE INDEX subject_versions_day ON subject_versions(subject_id, effective_day DESC, id DESC);
INSERT INTO subject_versions (subject_id, effective_day, name, color, sort_order, created_at, status)
SELECT id, date('now','+8 hours'), name, color, sort_order, created_at,
       CASE WHEN deleted=1 THEN 'deleted' WHEN archived=1 THEN 'stopped' ELSE 'active' END
FROM subjects;

-- 旧目标只有日与汇总内容，历史边界只有日期；都不会参与循环排程。
CREATE TABLE plan_versions (
    id INTEGER PRIMARY KEY,
    kind TEXT NOT NULL DEFAULT 'recurring' CHECK (kind IN ('recurring','legacy_target','history_boundary')),
    plan_id INTEGER,
    subject_id INTEGER,
    effective_day TEXT NOT NULL,
    start_time TEXT,
    duration_minutes INTEGER,
    break_minutes INTEGER,
    weekdays TEXT,
    color TEXT,
    note TEXT,
    status TEXT CHECK (status IN ('active','stopped','deleted')),
    legacy_targets_json TEXT,
    adoption_day TEXT,
    CHECK (
        (kind='recurring' AND plan_id IS NOT NULL AND subject_id IS NOT NULL
            AND start_time IS NOT NULL AND duration_minutes IS NOT NULL AND break_minutes IS NOT NULL
            AND weekdays IS NOT NULL AND color IS NOT NULL AND note IS NOT NULL
            AND status IS NOT NULL AND legacy_targets_json IS NULL AND adoption_day IS NULL)
        OR
        (kind<>'recurring' AND plan_id IS NULL AND subject_id IS NULL AND start_time IS NULL
            AND duration_minutes IS NULL AND break_minutes IS NULL AND weekdays IS NULL
            AND color IS NULL AND note IS NULL AND status IS NULL AND (
                (kind='legacy_target' AND legacy_targets_json IS NOT NULL
                    AND json_valid(legacy_targets_json) AND adoption_day IS NULL)
                OR (kind='history_boundary' AND legacy_targets_json IS NULL)
            ))
    )
);
CREATE INDEX plan_versions_day ON plan_versions(plan_id, effective_day DESC, id DESC) WHERE kind='recurring';
CREATE INDEX plan_versions_subject ON plan_versions(subject_id) WHERE kind='recurring';
CREATE UNIQUE INDEX plan_versions_legacy_day ON plan_versions(effective_day) WHERE kind='legacy_target';
CREATE UNIQUE INDEX plan_versions_boundary ON plan_versions(kind) WHERE kind='history_boundary';

WITH migration_day AS (
    SELECT date('now','+8 hours') AS today,
           EXISTS(SELECT 1 FROM plans UNION ALL SELECT 1 FROM sessions
                  UNION ALL SELECT 1 FROM daily_plan_targets) AS has_history
)
INSERT INTO plan_versions (kind, effective_day, adoption_day)
SELECT 'history_boundary', CASE WHEN has_history THEN date(today,'+1 day') ELSE today END,
       CASE WHEN has_history THEN today END
FROM migration_day;

INSERT INTO plan_versions (plan_id, subject_id, effective_day, start_time, duration_minutes, break_minutes, weekdays, color, note, status)
SELECT p.id, p.subject_id, date('now','+8 hours'), p.start_time, p.duration_minutes, p.break_minutes, p.weekdays, p.color, p.note,
       CASE WHEN p.deleted=1 THEN 'deleted' WHEN p.active=0 AND s.archived=0 THEN 'stopped' ELSE 'active' END
FROM plans p JOIN subjects s ON s.id=p.subject_id;

INSERT INTO plan_versions (kind, effective_day, legacy_targets_json)
SELECT 'legacy_target', day, targets_json FROM daily_plan_targets;

CREATE TABLE sessions_v2 (
    id INTEGER PRIMARY KEY,
    subject_id INTEGER NOT NULL,
    plan_id INTEGER,
    start_at TEXT NOT NULL,
    planned_end_at TEXT,
    end_at TEXT,
    paused_at TEXT,
    resumed_at TEXT,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'timer',
    note TEXT NOT NULL DEFAULT '',
    deleted INTEGER NOT NULL DEFAULT 0,
    break_started_at TEXT,
    break_notified INTEGER NOT NULL DEFAULT 0,
    break_used_seconds INTEGER NOT NULL DEFAULT 0,
    plan_version_id INTEGER REFERENCES plan_versions(id) ON DELETE SET NULL
);
-- 仅未结束的计时可继承升级时唯一已知的计划版本。更早的计时保留原关联。
INSERT INTO sessions_v2 (id, subject_id, plan_id, start_at, planned_end_at, end_at, paused_at,
    resumed_at, duration_seconds, source, note, deleted, break_started_at, break_notified, break_used_seconds, plan_version_id)
SELECT s.id, s.subject_id, s.plan_id, s.start_at, s.planned_end_at, s.end_at, s.paused_at,
       s.resumed_at, s.duration_seconds, s.source, s.note, s.deleted, s.break_started_at, s.break_notified, s.break_used_seconds,
       CASE WHEN s.end_at IS NULL AND s.deleted=0 THEN (
           SELECT p.id FROM plan_versions p WHERE p.kind='recurring' AND p.plan_id=s.plan_id
       ) END
FROM sessions s;
DROP TABLE sessions;
ALTER TABLE sessions_v2 RENAME TO sessions;
DROP TABLE plans;
DROP TABLE subjects;
DROP TABLE daily_plan_targets;

-- 旧表只含已填写的追因，完整保留原时长、标准和原因。
ALTER TABLE daily_reviews ADD COLUMN status TEXT NOT NULL DEFAULT 'submitted'
    CHECK (status IN ('passed','pending','submitted'));
