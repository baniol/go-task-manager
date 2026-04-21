package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLite) ImportReplace(ctx context.Context, payload ImportPayload) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Wipe — order matters for FKs, even though ON DELETE CASCADE would cover it.
	for _, stmt := range []string{
		`DELETE FROM time_entries`,
		`DELETE FROM task_tags`,
		`DELETE FROM tasks`,
		`DELETE FROM tags`,
		`DELETE FROM sqlite_sequence`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("wipe: %w", err)
		}
	}

	// Tags.
	tagIDs := map[string]int64{}
	ensureTagCached := func(name string) (int64, error) {
		if id, ok := tagIDs[name]; ok {
			return id, nil
		}
		id, err := ensureTag(ctx, tx, name)
		if err != nil {
			return 0, err
		}
		tagIDs[name] = id
		return id, nil
	}
	for _, name := range payload.Tags {
		if _, err := ensureTagCached(name); err != nil {
			return err
		}
	}

	// Tasks + task_tags + time_entries, preserving IDs.
	for _, t := range payload.Tasks {
		var dueStr sql.NullString
		if t.DueAt != nil {
			dueStr = sql.NullString{String: t.DueAt.UTC().Format(time.RFC3339), Valid: true}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tasks (id, title, body, status, priority, draft, due_at, position, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.Title, t.Body, t.Status, t.Priority,
			boolToInt(t.Draft), dueStr, t.Position, t.CreatedAt.UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("insert task %d: %w", t.ID, err)
		}
		for _, name := range dedupeStrings(t.Tags) {
			tagID, err := ensureTagCached(name)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)`, t.ID, tagID); err != nil {
				return fmt.Errorf("link tag %q to task %d: %w", name, t.ID, err)
			}
		}
		for _, e := range t.TimeEntries {
			var endedStr sql.NullString
			if e.EndedAt != nil {
				endedStr = sql.NullString{String: e.EndedAt.UTC().Format(time.RFC3339), Valid: true}
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO time_entries (id, task_id, started_at, ended_at, note)
				 VALUES (?, ?, ?, ?, ?)`,
				e.ID, t.ID, e.StartedAt.UTC().Format(time.RFC3339), endedStr, e.Note,
			); err != nil {
				return fmt.Errorf("insert time entry %d: %w", e.ID, err)
			}
		}
	}

	return tx.Commit()
}
