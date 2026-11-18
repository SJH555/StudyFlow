-- +goose Up

CREATE TABLE subjects (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, color TEXT NOT NULL DEFAULT '#007AFF', created_at TEXT NOT NULL, archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0, sort_order INTEGER);

CREATE TABLE plans (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), start_time TEXT NOT NULL, duration_minutes INTEGER NOT NULL, weekdays TEXT NOT NULL DEFAULT '1,2,3,4,5,6,0', active INTEGER NOT NULL DEFAULT 1, color TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_minutes INTEGER NOT NULL DEFAULT 0, note TEXT NOT NULL DEFAULT '');

CREATE TABLE sessions (id INTEGER PRIMARY KEY, subject_id INTEGER NOT NULL REFERENCES subjects(id), plan_id INTEGER, start_at TEXT NOT NULL, planned_end_at TEXT, end_at TEXT, paused_at TEXT, resumed_at TEXT, duration_seconds INTEGER NOT NULL DEFAULT 0, source TEXT NOT NULL DEFAULT 'timer', note TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0, break_started_at TEXT, break_notified INTEGER NOT NULL DEFAULT 0, break_used_seconds INTEGER NOT NULL DEFAULT 0);

CREATE TABLE activity_logs (id INTEGER PRIMARY KEY, happened_at TEXT NOT NULL, content TEXT NOT NULL);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);

CREATE TABLE daily_reviews (day TEXT PRIMARY KEY, mode TEXT NOT NULL, threshold INTEGER NOT NULL, planned_seconds INTEGER NOT NULL, actual_seconds INTEGER NOT NULL, submitted_at TEXT NOT NULL);

CREATE TABLE daily_review_items (id INTEGER PRIMARY KEY, day TEXT NOT NULL REFERENCES daily_reviews(day) ON DELETE CASCADE, subject_id INTEGER, subject_name TEXT NOT NULL, planned_seconds INTEGER NOT NULL, actual_seconds INTEGER NOT NULL, reason TEXT NOT NULL);

CREATE TABLE daily_plan_targets (day TEXT PRIMARY KEY, targets_json TEXT NOT NULL);

CREATE TABLE stage_goals (id INTEGER PRIMARY KEY, title TEXT NOT NULL, deadline TEXT NOT NULL, note TEXT NOT NULL DEFAULT '', completed_at TEXT, accepted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, deleted INTEGER NOT NULL DEFAULT 0);

-- Initial settings belong to first creation only; adopting old data skips this file.
INSERT INTO settings(key,value) VALUES
 ('dailyReviewStartDate',date('now','localtime')),
 ('dailyReviewEnabled','0');
