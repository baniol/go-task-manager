CREATE TABLE tasks (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('todo','doing','action','done')),
    created_at TEXT NOT NULL,
    priority   TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high')),
    body       TEXT NOT NULL DEFAULT '',
    due_at     TEXT,
    position   INTEGER NOT NULL DEFAULT 0,
    draft      INTEGER NOT NULL DEFAULT 0 CHECK (draft IN (0, 1))
);

CREATE TABLE tags (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE task_tags (
    task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id  INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (task_id, tag_id)
);

CREATE INDEX idx_task_tags_tag ON task_tags(tag_id);

-- Time tracking: each entry is a period (started_at, ended_at).
-- ended_at IS NULL means the timer is running.
-- Only one running timer globally, enforced by a partial unique index on a constant.
CREATE TABLE time_entries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    started_at TEXT NOT NULL,
    ended_at   TEXT,
    note       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_time_entries_task ON time_entries(task_id);
CREATE INDEX idx_time_entries_started_at ON time_entries(started_at);
CREATE UNIQUE INDEX idx_time_entries_one_active
    ON time_entries((1)) WHERE ended_at IS NULL;

-- FTS5 full-text search across task title and body, synced by triggers.
CREATE VIRTUAL TABLE tasks_fts USING fts5(
    title,
    body,
    content='tasks',
    content_rowid='id'
);

CREATE TRIGGER tasks_ai AFTER INSERT ON tasks BEGIN
    INSERT INTO tasks_fts(rowid, title, body) VALUES (new.id, new.title, COALESCE(new.body, ''));
END;

CREATE TRIGGER tasks_ad AFTER DELETE ON tasks BEGIN
    INSERT INTO tasks_fts(tasks_fts, rowid, title, body) VALUES('delete', old.id, old.title, COALESCE(old.body, ''));
END;

CREATE TRIGGER tasks_au AFTER UPDATE ON tasks BEGIN
    INSERT INTO tasks_fts(tasks_fts, rowid, title, body) VALUES('delete', old.id, old.title, COALESCE(old.body, ''));
    INSERT INTO tasks_fts(rowid, title, body) VALUES (new.id, new.title, COALESCE(new.body, ''));
END;
