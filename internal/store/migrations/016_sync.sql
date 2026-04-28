-- Sync foundations: stable cross-device identity (uuid), modification timestamp
-- (updated_at) for last-write-wins merging, and tombstones (deleted_at) so deletes
-- propagate. The local INTEGER id stays for display ("#48") and is per-database.

ALTER TABLE tasks ADD COLUMN uuid       TEXT;
ALTER TABLE tasks ADD COLUMN updated_at TEXT;
ALTER TABLE tasks ADD COLUMN deleted_at TEXT;

UPDATE tasks SET updated_at = created_at WHERE updated_at IS NULL;

-- SQLite treats NULLs as distinct in UNIQUE indexes, so the index is safe even
-- before existing rows are backfilled in Go after migration completes.
CREATE UNIQUE INDEX idx_tasks_uuid ON tasks(uuid);
