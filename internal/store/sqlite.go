package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"go-task-manager/internal/task"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLite struct {
	db   *sql.DB
	path string
}

func OpenSQLite(ctx context.Context, path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := backfillTaskUUIDs(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill uuids: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign_keys: %w", err)
	}
	return &SQLite{db: db, path: path}, nil
}

// backfillTaskUUIDs assigns a UUID to any pre-sync task that still has NULL uuid.
// One-shot upgrade path: new inserts always provide uuid, so this is a no-op
// after the first run on an old database.
func backfillTaskUUIDs(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT id FROM tasks WHERE uuid IS NULL`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tasks SET uuid = ? WHERE id = ?`, uuid.NewString(), id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) Path() string { return s.path }

// Backup takes a consistent snapshot of the database to dst via VACUUM INTO.
// The destination file must not exist — SQLite requires that.
func (s *SQLite) Backup(ctx context.Context, dst string) error {
	if dst == "" {
		return errors.New("backup: empty destination path")
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return fmt.Errorf("vacuum into %s: %w", dst, err)
	}
	return nil
}

// migrate runs pending migrations with FK disabled — otherwise DROP TABLE in
// the 12-step ALTER cascades ON DELETE to task_tags / time_entries and wipes data.
// PRAGMA foreign_keys is a no-op inside a transaction, so we disable it outside the TX loop.
// After migrating we run foreign_key_check as a sanity check; FK is re-enabled in OpenSQLite.
func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign_keys: %w", err)
	}
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	applied := 0
	for _, name := range names {
		version, err := parseMigrationVersion(name)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if version <= current {
			continue
		}
		applied++
		body, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("bump user_version to %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
	}

	if applied == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	defer rows.Close()
	var violations []string
	for rows.Next() {
		var table, parent sql.NullString
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("scan fk violation: %w", err)
		}
		violations = append(violations, fmt.Sprintf("%s rowid=%d -> %s", table.String, rowid.Int64, parent.String))
	}
	if len(violations) > 0 {
		return fmt.Errorf("foreign_key_check failed: %s", strings.Join(violations, "; "))
	}
	return nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) Add(ctx context.Context, in AddInput) (task.Task, error) {
	if in.Title == "" {
		return task.Task{}, errors.New("title is required")
	}
	if in.Priority == "" {
		in.Priority = task.PriorityNormal
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	draft := !in.Ready
	taskUUID := uuid.NewString()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return task.Task{}, err
	}
	defer tx.Rollback()

	var dueStr sql.NullString
	if in.Due != nil {
		dueStr = sql.NullString{String: in.Due.UTC().Format(time.RFC3339), Valid: true}
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO tasks (uuid, title, body, status, priority, draft, due_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskUUID, in.Title, in.Body, task.StatusTodo, in.Priority, boolToInt(draft), dueStr, nowStr, nowStr,
	)
	if err != nil {
		return task.Task{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return task.Task{}, err
	}

	tags, err := setTaskTags(ctx, tx, id, dedupeStrings(in.Tags))
	if err != nil {
		return task.Task{}, err
	}

	if err := tx.Commit(); err != nil {
		return task.Task{}, err
	}

	var dueOut *time.Time
	if in.Due != nil {
		v := in.Due.UTC()
		dueOut = &v
	}
	return task.Task{
		ID:        id,
		UUID:      taskUUID,
		Title:     in.Title,
		Body:      in.Body,
		Status:    task.StatusTodo,
		Priority:  in.Priority,
		Tags:      tags,
		Draft:     draft,
		DueAt:     dueOut,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *SQLite) Get(ctx context.Context, id int64) (task.Task, error) {
	tasks, err := s.queryTasks(ctx, `WHERE t.id = ? AND t.deleted_at IS NULL`, []any{id}, SortDefault)
	if err != nil {
		return task.Task{}, err
	}
	if len(tasks) == 0 {
		return task.Task{}, fmt.Errorf("task %d not found", id)
	}
	return tasks[0], nil
}

func (s *SQLite) List(ctx context.Context, filter ListFilter) ([]task.Task, error) {
	var (
		where = []string{"t.deleted_at IS NULL"}
		args  []any
	)
	if filter.Status != "" {
		where = append(where, "t.status = ?")
		args = append(args, filter.Status)
	} else if len(filter.Statuses) > 0 {
		placeholders := strings.Repeat("?,", len(filter.Statuses))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, "t.status IN ("+placeholders+")")
		for _, st := range filter.Statuses {
			args = append(args, st)
		}
	}
	if filter.Priority != "" {
		where = append(where, "t.priority = ?")
		args = append(args, filter.Priority)
	}
	switch filter.DraftMode {
	case DraftOnly:
		where = append(where, "t.draft = 1")
	case DraftHide:
		where = append(where, "t.draft = 0")
	}
	if filter.NoDue {
		where = append(where, "t.due_at IS NULL")
	}
	if filter.Overdue {
		where = append(where,
			"t.due_at IS NOT NULL AND t.due_at < ? AND t.status != ?")
		args = append(args, time.Now().UTC().Format(time.RFC3339), task.StatusDone)
	}
	tags := dedupeStrings(filter.Tags)
	if len(tags) > 0 {
		placeholders := strings.Repeat("?,", len(tags))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where,
			`t.id IN (
				SELECT tt.task_id FROM task_tags tt
				JOIN tags tg ON tg.id = tt.tag_id
				WHERE tg.name IN (`+placeholders+`)
				GROUP BY tt.task_id
				HAVING COUNT(DISTINCT tg.id) = ?
			)`)
		for _, name := range tags {
			args = append(args, name)
		}
		args = append(args, len(tags))
	}

	cond := ""
	if len(where) > 0 {
		cond = "WHERE " + strings.Join(where, " AND ")
	}
	return s.queryTasks(ctx, cond, args, filter.Sort)
}

// queryTasks sorts by the given order and loads tags with a separate query.
func (s *SQLite) queryTasks(ctx context.Context, cond string, args []any, order SortOrder) ([]task.Task, error) {
	priorityOrder := `CASE t.priority WHEN 'high' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END`
	orderBy := priorityOrder + `, t.id`
	switch order {
	case SortDue:
		// NULLS LAST: tasks without due land at the end.
		orderBy = `(t.due_at IS NULL), t.due_at, ` + priorityOrder + `, t.id`
	case SortPosition:
		// Tasks with position > 0 on top (by position), the rest by priority/id.
		orderBy = `(t.position = 0), t.position, ` + priorityOrder + `, t.id`
	}
	query := `
		SELECT t.id, t.uuid, t.title, t.body, t.status, t.priority,
		       t.draft, t.due_at, t.position, t.created_at, t.updated_at, t.deleted_at
		FROM tasks t
		` + cond + `
		ORDER BY ` + orderBy

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		tasks []task.Task
		ids   []int64
	)
	for rows.Next() {
		var (
			t         task.Task
			uuidStr   sql.NullString
			draftInt  int
			dueAt     sql.NullString
			createdAt string
			updatedAt sql.NullString
			deletedAt sql.NullString
		)
		if err := rows.Scan(&t.ID, &uuidStr, &t.Title, &t.Body, &t.Status, &t.Priority,
			&draftInt, &dueAt, &t.Position, &createdAt, &updatedAt, &deletedAt); err != nil {
			return nil, err
		}
		t.UUID = uuidStr.String
		t.Draft = draftInt == 1
		t.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at for task %d: %w", t.ID, err)
		}
		if updatedAt.Valid {
			ts, err := time.Parse(time.RFC3339, updatedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse updated_at for task %d: %w", t.ID, err)
			}
			t.UpdatedAt = ts
		} else {
			t.UpdatedAt = t.CreatedAt
		}
		if deletedAt.Valid {
			ts, err := time.Parse(time.RFC3339, deletedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse deleted_at for task %d: %w", t.ID, err)
			}
			t.DeletedAt = &ts
		}
		if dueAt.Valid {
			due, err := time.Parse(time.RFC3339, dueAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse due_at for task %d: %w", t.ID, err)
			}
			t.DueAt = &due
		}
		tasks = append(tasks, t)
		ids = append(ids, t.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ids) == 0 {
		return tasks, nil
	}
	tagsByTask, err := loadTagsForTasks(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	for i := range tasks {
		tasks[i].Tags = tagsByTask[tasks[i].ID]
	}
	return tasks, nil
}

func (s *SQLite) Search(ctx context.Context, query string, filter SearchFilter) ([]task.Task, error) {
	if query == "" {
		return nil, errors.New("search query is required")
	}

	// FTS5 MATCH with prefix matching — each word gets a trailing `*`
	// so "bac" matches "backend". Queries with FTS5 operators (quotes,
	// OR, NOT, *) are passed through unchanged.
	ftsQuery := fts5PrefixQuery(query)

	where := []string{
		"t.deleted_at IS NULL",
		"t.id IN (SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH ?)",
	}
	args := []any{ftsQuery}

	if filter.Status != "" {
		where = append(where, "t.status = ?")
		args = append(args, filter.Status)
	} else if len(filter.Statuses) > 0 {
		placeholders := strings.Repeat("?,", len(filter.Statuses))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, "t.status IN ("+placeholders+")")
		for _, st := range filter.Statuses {
			args = append(args, st)
		}
	}
	tags := dedupeStrings(filter.Tags)
	if len(tags) > 0 {
		placeholders := strings.Repeat("?,", len(tags))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where,
			`t.id IN (
				SELECT tt.task_id FROM task_tags tt
				JOIN tags tg ON tg.id = tt.tag_id
				WHERE tg.name IN (`+placeholders+`)
				GROUP BY tt.task_id
				HAVING COUNT(DISTINCT tg.id) = ?
			)`)
		for _, name := range tags {
			args = append(args, name)
		}
		args = append(args, len(tags))
	}

	cond := "WHERE " + strings.Join(where, " AND ")
	return s.queryTasks(ctx, cond, args, SortDefault)
}

func (s *SQLite) Move(ctx context.Context, id int64, status task.Status) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		status, nowRFC3339(), id)
	if err != nil {
		return err
	}
	return checkAffected(res, id)
}

func (s *SQLite) Update(ctx context.Context, id int64, in EditInput) error {
	if in.Title == nil && in.Body == nil && in.Priority == nil && in.Draft == nil && in.Due == nil && !in.ClearDue {
		return errors.New("nothing to update")
	}
	if in.Title != nil && *in.Title == "" {
		return errors.New("title cannot be empty")
	}
	if in.Due != nil && in.ClearDue {
		return errors.New("cannot set and clear due at the same time")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := assertTaskExists(ctx, tx, id); err != nil {
		return err
	}

	var (
		sets []string
		args []any
	)
	if in.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *in.Title)
	}
	if in.Body != nil {
		sets = append(sets, "body = ?")
		args = append(args, *in.Body)
	}
	if in.Priority != nil {
		sets = append(sets, "priority = ?")
		args = append(args, *in.Priority)
	}
	if in.Draft != nil {
		sets = append(sets, "draft = ?")
		args = append(args, boolToInt(*in.Draft))
	}
	switch {
	case in.Due != nil:
		sets = append(sets, "due_at = ?")
		args = append(args, in.Due.UTC().Format(time.RFC3339))
	case in.ClearDue:
		sets = append(sets, "due_at = NULL")
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, nowRFC3339())
	args = append(args, id)
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLite) AddTaskTags(ctx context.Context, taskID int64, tags []string) error {
	tags = dedupeStrings(tags)
	if len(tags) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := assertTaskExists(ctx, tx, taskID); err != nil {
		return err
	}
	for _, name := range tags {
		tagID, err := ensureTag(ctx, tx, name)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO task_tags (task_id, tag_id) VALUES (?, ?)`, taskID, tagID); err != nil {
			return err
		}
	}
	if err := bumpTaskUpdatedAt(ctx, tx, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) RemoveTaskTags(ctx context.Context, taskID int64, tags []string) error {
	tags = dedupeStrings(tags)
	if len(tags) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := assertTaskExists(ctx, tx, taskID); err != nil {
		return err
	}
	placeholders := strings.Repeat("?,", len(tags))
	placeholders = placeholders[:len(placeholders)-1]
	args := []any{taskID}
	for _, name := range tags {
		args = append(args, name)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM task_tags WHERE task_id = ? AND tag_id IN (
			SELECT id FROM tags WHERE name IN (`+placeholders+`)
		)`, args...); err != nil {
		return err
	}
	if err := bumpTaskUpdatedAt(ctx, tx, taskID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) DeleteTag(ctx context.Context, name string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var tagID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID); err != nil {
		return 0, fmt.Errorf("tag %q not found", name)
	}
	var count int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_tags WHERE tag_id = ?`, tagID).Scan(&count); err != nil {
		return 0, err
	}
	// Bump updated_at on every task that had this tag, so tag removal propagates via sync.
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET updated_at = ?
		 WHERE id IN (SELECT task_id FROM task_tags WHERE tag_id = ?)
		   AND deleted_at IS NULL`,
		nowRFC3339(), tagID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, tagID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

// Delete is a soft delete: it sets deleted_at (tombstone) instead of removing
// the row, so the deletion can propagate through sync. Time entries and tag
// links are preserved; reads filter tombstoned rows. A running timer on the
// deleted task is stopped — leaving it active would be a stuck-state footgun.
func (s *SQLite) Delete(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := nowRFC3339()
	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		now, now, id)
	if err != nil {
		return err
	}
	if err := checkAffected(res, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE time_entries SET ended_at = ? WHERE task_id = ? AND ended_at IS NULL`,
		now, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) SetPositions(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE tasks SET position = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := nowRFC3339()
	for i, id := range ids {
		if _, err := stmt.ExecContext(ctx, i+1, now, id); err != nil {
			return fmt.Errorf("set position for task %d: %w", id, err)
		}
	}
	return tx.Commit()
}

func (s *SQLite) Publish(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET draft = 0, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		nowRFC3339(), id)
	if err != nil {
		return err
	}
	return checkAffected(res, id)
}

func (s *SQLite) Tags(ctx context.Context) ([]string, error) { return s.listNames(ctx, "tags") }

func (s *SQLite) listNames(ctx context.Context, table string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM `+table+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// fts5PrefixQuery appends `*` to each word so FTS5 does a prefix match.
// If the query contains FTS5 operators (quotes, OR, NOT, NEAR, *), returns it unchanged.
func fts5PrefixQuery(query string) string {
	// If the user wrote explicit FTS5 syntax, leave it alone.
	if strings.ContainsAny(query, `"*`) ||
		strings.Contains(query, " OR ") ||
		strings.Contains(query, " NOT ") ||
		strings.Contains(query, " NEAR") {
		return query
	}
	words := strings.Fields(query)
	for i, w := range words {
		words[i] = w + "*"
	}
	return strings.Join(words, " ")
}

// --- helpers ---

func ensureTag(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	return ensureRow(ctx, tx, "tags", name)
}

func ensureRow(ctx context.Context, tx *sql.Tx, table, name string) (int64, error) {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO `+table+` (name) VALUES (?)`, name); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM `+table+` WHERE name = ?`, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// setTaskTags replaces the task's tag set (full replace). Returns the final list of names.
func setTaskTags(ctx context.Context, tx *sql.Tx, taskID int64, names []string) ([]string, error) {
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = ?`, taskID); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	for _, name := range names {
		tagID, err := ensureTag(ctx, tx, name)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO task_tags (task_id, tag_id) VALUES (?, ?)`, taskID, tagID); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func loadTagsForTasks(ctx context.Context, db *sql.DB, ids []int64) (map[int64][]string, error) {
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.QueryContext(ctx, `
		SELECT tt.task_id, tg.name
		FROM task_tags tt
		JOIN tags tg ON tg.id = tt.tag_id
		WHERE tt.task_id IN (`+placeholders+`)
		ORDER BY tg.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]string)
	for rows.Next() {
		var (
			taskID int64
			name   string
		)
		if err := rows.Scan(&taskID, &name); err != nil {
			return nil, err
		}
		out[taskID] = append(out[taskID], name)
	}
	return out, rows.Err()
}

func assertTaskExists(ctx context.Context, tx *sql.Tx, id int64) error {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tasks WHERE id = ? AND deleted_at IS NULL`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	return nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// parseMigrationVersion extracts the leading integer from a migration filename
// like "016_sync.sql" — the version is encoded in the filename, not derived
// from sort order, so version numbers can jump (e.g. when older migrations are
// squashed into a newer baseline).
func parseMigrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("missing version prefix in %q", name)
	}
	v, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("non-numeric version prefix in %q: %w", name, err)
	}
	return v, nil
}

func bumpTaskUpdatedAt(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		nowRFC3339(), id)
	return err
}

func checkAffected(res sql.Result, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
