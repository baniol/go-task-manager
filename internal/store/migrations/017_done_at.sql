-- Track when a task last entered the done state, so the TUI's done tab
-- can sort by completion time. Backfill is approximate: existing done rows
-- get updated_at as a best-effort proxy (we don't have a status-change log).

ALTER TABLE tasks ADD COLUMN done_at TEXT;

UPDATE tasks SET done_at = updated_at WHERE status = 'done' AND done_at IS NULL;
