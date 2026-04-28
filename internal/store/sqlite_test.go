package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"go-task-manager/internal/task"
)

func newTestStore(t *testing.T) *SQLite {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustAdd(t *testing.T, s *SQLite, in AddInput) task.Task {
	t.Helper()
	got, err := s.Add(context.Background(), in)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	return got
}

func TestAddSetsFieldsAndDefaults(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	got, err := s.Add(ctx, AddInput{Title: "write README", Priority: task.PriorityHigh})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got.ID == 0 {
		t.Error("want non-zero ID")
	}
	if got.Title != "write README" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Status != task.StatusTodo {
		t.Errorf("Status = %q, want todo (default)", got.Status)
	}
	if got.Priority != task.PriorityHigh {
		t.Errorf("Priority = %q, want high", got.Priority)
	}
	if !got.Draft {
		t.Errorf("Draft = false, want true (default)")
	}
	if got.Body != "" {
		t.Errorf("Body = %q, want empty default", got.Body)
	}
	if len(got.Tags) != 0 {
		t.Errorf("Tags = %v, want empty default", got.Tags)
	}
	if got.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test start %v", got.CreatedAt, before)
	}
}

func TestAddWithAllFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.Add(ctx, AddInput{
		Title:    "feature X",
		Body:     "more details",
		Priority: task.PriorityHigh,
		Tags:     []string{"backend", "api", "backend"}, // duplicate ignored
		Ready:    true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got.Body != "more details" {
		t.Errorf("Body = %q", got.Body)
	}
	if got.Draft {
		t.Errorf("Draft = true, want false (Ready=true)")
	}

	// re-read from DB confirms tags and dedup
	fetched, err := s.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	wantTags := []string{"api", "backend"}
	gotTags := append([]string(nil), fetched.Tags...)
	sort.Strings(gotTags)
	if !reflect.DeepEqual(gotTags, wantTags) {
		t.Errorf("Tags = %v, want %v", gotTags, wantTags)
	}
}

func TestListSortsByPriorityThenID(t *testing.T) {
	s := newTestStore(t)

	mustAdd(t, s, AddInput{Title: "a-low", Priority: task.PriorityLow})
	mustAdd(t, s, AddInput{Title: "b-high", Priority: task.PriorityHigh})
	mustAdd(t, s, AddInput{Title: "c-normal", Priority: task.PriorityNormal})
	mustAdd(t, s, AddInput{Title: "d-high", Priority: task.PriorityHigh})

	tasks, err := s.List(context.Background(), ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantTitles := []string{"b-high", "d-high", "c-normal", "a-low"}
	if len(tasks) != len(wantTitles) {
		t.Fatalf("len = %d, want %d", len(tasks), len(wantTitles))
	}
	for i, want := range wantTitles {
		if tasks[i].Title != want {
			t.Errorf("tasks[%d].Title = %q, want %q", i, tasks[i].Title, want)
		}
	}
}

func TestListFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := mustAdd(t, s, AddInput{Title: "a-high-todo", Priority: task.PriorityHigh})
	b := mustAdd(t, s, AddInput{Title: "b-high-doing", Priority: task.PriorityHigh})
	c := mustAdd(t, s, AddInput{Title: "c-low-todo", Priority: task.PriorityLow})
	if err := s.Move(ctx, b.ID, task.StatusDoing); err != nil {
		t.Fatal(err)
	}

	highs, err := s.List(ctx, ListFilter{Priority: task.PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	if len(highs) != 2 || highs[0].ID != a.ID || highs[1].ID != b.ID {
		t.Errorf("priority=high filter: got %+v", highs)
	}

	todos, err := s.List(ctx, ListFilter{Status: task.StatusTodo})
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 2 || todos[0].ID != a.ID || todos[1].ID != c.ID {
		t.Errorf("status=todo filter: got %+v", todos)
	}

	both, err := s.List(ctx, ListFilter{Status: task.StatusTodo, Priority: task.PriorityHigh})
	if err != nil {
		t.Fatal(err)
	}
	if len(both) != 1 || both[0].ID != a.ID {
		t.Errorf("combined filter: got %+v", both)
	}
}

func TestListTagFilterIsAndSemantics(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := mustAdd(t, s, AddInput{Title: "a", Priority: task.PriorityNormal, Tags: []string{"backend", "api"}})
	mustAdd(t, s, AddInput{Title: "b", Priority: task.PriorityNormal, Tags: []string{"backend"}})
	mustAdd(t, s, AddInput{Title: "c", Priority: task.PriorityNormal, Tags: []string{"frontend"}})

	got, err := s.List(ctx, ListFilter{Tags: []string{"backend", "api"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("AND filter on backend+api: got %+v, want only %d", got, a.ID)
	}
}

func TestListDraftMode(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	draft := mustAdd(t, s, AddInput{Title: "draft", Priority: task.PriorityNormal})
	ready := mustAdd(t, s, AddInput{Title: "ready", Priority: task.PriorityNormal, Ready: true})

	all, err := s.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("DraftAll: got %d tasks, want 2", len(all))
	}

	onlyDraft, err := s.List(ctx, ListFilter{DraftMode: DraftOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyDraft) != 1 || onlyDraft[0].ID != draft.ID {
		t.Errorf("DraftOnly: got %+v, want only %d", onlyDraft, draft.ID)
	}

	onlyReady, err := s.List(ctx, ListFilter{DraftMode: DraftHide})
	if err != nil {
		t.Fatal(err)
	}
	if len(onlyReady) != 1 || onlyReady[0].ID != ready.ID {
		t.Errorf("DraftHide: got %+v, want only %d", onlyReady, ready.ID)
	}
}

func TestAddStoresDueAtAsUTC(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	local := time.Date(2026, 5, 10, 18, 0, 0, 0, time.Local)
	got, err := s.Add(ctx, AddInput{Title: "x", Priority: task.PriorityNormal, Due: &local})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got.DueAt == nil {
		t.Fatalf("DueAt nil after Add")
	}
	if !got.DueAt.Equal(local) {
		t.Errorf("DueAt = %v, want equal to %v", got.DueAt, local)
	}

	// Roundtrip: the DB value must be in UTC (RFC3339 Z).
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT due_at FROM tasks WHERE id = ?`, got.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(raw, "Z") {
		t.Errorf("due_at = %q, want UTC (suffix Z)", raw)
	}

	fetched, err := s.Get(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.DueAt == nil || !fetched.DueAt.Equal(local) {
		t.Errorf("roundtrip DueAt = %v, want %v", fetched.DueAt, local)
	}
}

func TestUpdateDueSetAndClear(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{Title: "x", Priority: task.PriorityNormal})
	due := time.Date(2026, 6, 1, 23, 59, 59, 0, time.Local)

	if err := s.Update(ctx, added.ID, EditInput{Due: &due}); err != nil {
		t.Fatalf("Update set due: %v", err)
	}
	got, _ := s.Get(ctx, added.ID)
	if got.DueAt == nil || !got.DueAt.Equal(due) {
		t.Errorf("after set: DueAt = %v, want %v", got.DueAt, due)
	}

	if err := s.Update(ctx, added.ID, EditInput{ClearDue: true}); err != nil {
		t.Fatalf("Update clear due: %v", err)
	}
	got, _ = s.Get(ctx, added.ID)
	if got.DueAt != nil {
		t.Errorf("after clear: DueAt = %v, want nil", got.DueAt)
	}
}

func TestUpdateDueAndClearConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{Title: "x", Priority: task.PriorityNormal})
	due := time.Now()
	if err := s.Update(ctx, added.ID, EditInput{Due: &due, ClearDue: true}); err == nil {
		t.Error("want error when Due and ClearDue both set")
	}
}

func TestListOverdueExcludesDone(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	overdueTodo := mustAdd(t, s, AddInput{Title: "overdue", Priority: task.PriorityNormal, Due: &past})
	mustAdd(t, s, AddInput{Title: "future", Priority: task.PriorityNormal, Due: &future})
	mustAdd(t, s, AddInput{Title: "no-due", Priority: task.PriorityNormal})
	overdueDone := mustAdd(t, s, AddInput{Title: "overdue-done", Priority: task.PriorityNormal, Due: &past})
	if err := s.Move(ctx, overdueDone.ID, task.StatusDone); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx, ListFilter{Overdue: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != overdueTodo.ID {
		t.Errorf("Overdue: got %+v, want only id=%d", got, overdueTodo.ID)
	}
}

func TestListNoDue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	due := time.Now().Add(24 * time.Hour)
	mustAdd(t, s, AddInput{Title: "has-due", Priority: task.PriorityNormal, Due: &due})
	nd := mustAdd(t, s, AddInput{Title: "no-due", Priority: task.PriorityNormal})

	got, err := s.List(ctx, ListFilter{NoDue: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != nd.ID {
		t.Errorf("NoDue: got %+v, want only id=%d", got, nd.ID)
	}
}

func TestListSortDueNullsLast(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	late := time.Now().Add(72 * time.Hour)
	early := time.Now().Add(24 * time.Hour)

	idNoDue := mustAdd(t, s, AddInput{Title: "no-due", Priority: task.PriorityHigh}).ID
	idLate := mustAdd(t, s, AddInput{Title: "late", Priority: task.PriorityLow, Due: &late}).ID
	idEarly := mustAdd(t, s, AddInput{Title: "early", Priority: task.PriorityNormal, Due: &early}).ID

	got, err := s.List(ctx, ListFilter{Sort: SortDue})
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int64{idEarly, idLate, idNoDue}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Errorf("got[%d].ID = %d, want %d", i, got[i].ID, want)
		}
	}
}

func TestMoveChangesStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{Title: "task", Priority: task.PriorityNormal})
	if err := s.Move(ctx, added.ID, task.StatusDoing); err != nil {
		t.Fatalf("Move: %v", err)
	}
	got, err := s.Get(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.StatusDoing {
		t.Errorf("Status = %q, want doing", got.Status)
	}
}

func TestMoveUnknownIDReturnsError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Move(context.Background(), 9999, task.StatusDone); err == nil {
		t.Error("want error for unknown id, got nil")
	}
}

func TestUpdateTitleAndBody(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{Title: "old", Priority: task.PriorityNormal})
	newTitle := "new"
	newBody := "longer description"
	if err := s.Update(ctx, added.ID, EditInput{Title: &newTitle, Body: &newBody}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Get(ctx, added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Body != "longer description" {
		t.Errorf("after Update: %+v", got)
	}
}

func TestAddAndRemoveTaskTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{
		Title: "x", Priority: task.PriorityNormal,
		Tags: []string{"a", "b"},
	})

	// add more tags
	if err := s.AddTaskTags(ctx, added.ID, []string{"c", "d"}); err != nil {
		t.Fatalf("AddTaskTags: %v", err)
	}
	got, _ := s.Get(ctx, added.ID)
	sort.Strings(got.Tags)
	if !reflect.DeepEqual(got.Tags, []string{"a", "b", "c", "d"}) {
		t.Errorf("after add tags: %v", got.Tags)
	}

	// remove some tags
	if err := s.RemoveTaskTags(ctx, added.ID, []string{"a", "c"}); err != nil {
		t.Fatalf("RemoveTaskTags: %v", err)
	}
	got, _ = s.Get(ctx, added.ID)
	sort.Strings(got.Tags)
	if !reflect.DeepEqual(got.Tags, []string{"b", "d"}) {
		t.Errorf("after remove tags: %v", got.Tags)
	}

	// adding duplicate tags is idempotent
	if err := s.AddTaskTags(ctx, added.ID, []string{"b"}); err != nil {
		t.Fatalf("AddTaskTags duplicate: %v", err)
	}
	got, _ = s.Get(ctx, added.ID)
	sort.Strings(got.Tags)
	if !reflect.DeepEqual(got.Tags, []string{"b", "d"}) {
		t.Errorf("after duplicate add: %v", got.Tags)
	}
}

func TestDeleteTagSystemWide(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, s, AddInput{Title: "a", Priority: task.PriorityNormal, Tags: []string{"shared"}})
	mustAdd(t, s, AddInput{Title: "b", Priority: task.PriorityNormal, Tags: []string{"shared", "other"}})

	n, err := s.DeleteTag(ctx, "shared")
	if err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if n != 2 {
		t.Errorf("unlinked = %d, want 2", n)
	}

	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tags, []string{"other"}) {
		t.Errorf("Tags after delete = %v, want [other]", tags)
	}
}

func TestAddEmptyTitleReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Add(context.Background(), AddInput{Title: ""})
	if err == nil {
		t.Error("want error for empty title, got nil")
	}
}

func TestUpdateNothingToUpdateReturnsError(t *testing.T) {
	s := newTestStore(t)
	added := mustAdd(t, s, AddInput{Title: "x", Priority: task.PriorityNormal})
	if err := s.Update(context.Background(), added.ID, EditInput{}); err == nil {
		t.Error("want error for empty EditInput, got nil")
	}
}

func TestUpdateUnknownIDReturnsError(t *testing.T) {
	s := newTestStore(t)
	title := "x"
	if err := s.Update(context.Background(), 9999, EditInput{Title: &title}); err == nil {
		t.Error("want error for unknown id, got nil")
	}
}

func TestPublishFlipsDraftToReady(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	added := mustAdd(t, s, AddInput{Title: "x", Priority: task.PriorityNormal})
	if !added.Draft {
		t.Fatalf("expected draft by default, got %+v", added)
	}
	if err := s.Publish(ctx, added.ID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, _ := s.Get(ctx, added.ID)
	if got.Draft {
		t.Errorf("after Publish: still draft")
	}
}

func TestDeleteSoftDeletesAndPreservesLinks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	keep := mustAdd(t, s, AddInput{Title: "keep", Priority: task.PriorityNormal})
	drop := mustAdd(t, s, AddInput{
		Title: "drop", Priority: task.PriorityNormal,
		Tags: []string{"shared"},
	})
	if err := s.Delete(ctx, drop.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	tasks, err := s.List(ctx, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != keep.ID {
		t.Errorf("tasks after delete = %+v, want only id=%d", tasks, keep.ID)
	}
	if _, err := s.Get(ctx, drop.ID); err == nil {
		t.Errorf("Get after Delete should fail for tombstoned task")
	}

	// Soft delete: row + tag links remain so sync can propagate the tombstone.
	var deletedAt sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT deleted_at FROM tasks WHERE id = ?`, drop.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("read deleted_at: %v", err)
	}
	if !deletedAt.Valid {
		t.Errorf("deleted_at is NULL, want non-null tombstone")
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_tags WHERE task_id = ?`, drop.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("task_tags rows for tombstoned task = %d, want 1 (preserved)", n)
	}
}

func TestDeleteUnknownIDReturnsError(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete(context.Background(), 9999); err == nil {
		t.Error("want error for unknown id, got nil")
	}
}

func TestTagsListing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, s, AddInput{Title: "a", Priority: task.PriorityNormal, Tags: []string{"backend"}})
	mustAdd(t, s, AddInput{Title: "b", Priority: task.PriorityNormal, Tags: []string{"api", "backend"}})

	tags, err := s.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantTags := []string{"api", "backend"}
	if !reflect.DeepEqual(tags, wantTags) {
		t.Errorf("Tags = %v, want %v", tags, wantTags)
	}
}

func TestMigrationsIdempotentOnReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")

	s1, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := s1.Add(ctx, AddInput{Title: "persist-me", Priority: task.PriorityHigh}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s1.Close()

	s2, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()

	var userVersion int
	if err := s2.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if userVersion != 16 {
		t.Errorf("user_version = %d, want 16", userVersion)
	}

	tasks, err := s2.List(ctx, ListFilter{})
	if err != nil {
		t.Fatalf("List after reopen: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1 (no duplicates from re-migrate)", len(tasks))
	}
	if tasks[0].Title != "persist-me" || tasks[0].Priority != task.PriorityHigh {
		t.Errorf("task roundtrip mismatch: %+v", tasks[0])
	}
}

func TestSearchBasic(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, s, AddInput{Title: "deploy backend service", Body: "run ansible playbook", Ready: true})
	mustAdd(t, s, AddInput{Title: "fix frontend bug", Body: "CSS alignment issue", Ready: true})
	mustAdd(t, s, AddInput{Title: "write docs", Body: "backend API reference", Ready: true})

	// Search for "backend" — should hit the first's title and the third's body.
	results, err := s.Search(ctx, "backend", SearchFilter{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	titles := []string{results[0].Title, results[1].Title}
	sort.Strings(titles)
	want := []string{"deploy backend service", "write docs"}
	if !reflect.DeepEqual(titles, want) {
		t.Errorf("titles = %v, want %v", titles, want)
	}
}

func TestSearchPrefixMatching(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	mustAdd(t, s, AddInput{Title: "deploy backend service", Ready: true})
	mustAdd(t, s, AddInput{Title: "fix frontend bug", Ready: true})

	// Prefix "back" should match "backend".
	results, err := s.Search(ctx, "back", SearchFilter{})
	if err != nil {
		t.Fatalf("Search prefix: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results for 'back', want 1", len(results))
	}
	if results[0].Title != "deploy backend service" {
		t.Errorf("title = %q", results[0].Title)
	}

	// Single letter "d" — should match "deploy".
	results, err = s.Search(ctx, "d", SearchFilter{})
	if err != nil {
		t.Fatalf("Search single char: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results for 'd', want 1", len(results))
	}

	// Manual FTS5 syntax (quotes) — do not append *.
	results, err = s.Search(ctx, `"deploy backend"`, SearchFilter{})
	if err != nil {
		t.Fatalf("Search phrase: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results for phrase, want 1", len(results))
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Search(context.Background(), "", SearchFilter{})
	if err == nil {
		t.Fatal("want error for empty query")
	}
}

func TestSearchWithFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	t1 := mustAdd(t, s, AddInput{Title: "deploy api", Tags: []string{"ops"}, Ready: true})
	mustAdd(t, s, AddInput{Title: "deploy frontend", Tags: []string{"web"}, Ready: true})
	mustAdd(t, s, AddInput{Title: "deploy monitoring", Tags: []string{"ops"}, Ready: true})

	// Filter by tag
	results, err := s.Search(ctx, "deploy", SearchFilter{Tags: []string{"ops"}})
	if err != nil {
		t.Fatalf("Search with tag: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("tag filter: got %d results, want 2", len(results))
	}

	// Filter by status — move one to doing
	if err := s.Move(ctx, t1.ID, task.StatusDoing); err != nil {
		t.Fatalf("Move: %v", err)
	}
	results, err = s.Search(ctx, "deploy", SearchFilter{Status: task.StatusDoing})
	if err != nil {
		t.Fatalf("Search with status: %v", err)
	}
	if len(results) != 1 || results[0].ID != t1.ID {
		t.Errorf("status filter: got %d results, want 1", len(results))
	}
}

func TestSearchFTSSyncOnUpdateAndDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tk := mustAdd(t, s, AddInput{Title: "original title", Ready: true})

	// After update, title should be findable under the new value.
	newTitle := "updated title unique"
	if err := s.Update(ctx, tk.ID, EditInput{Title: &newTitle}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	results, err := s.Search(ctx, "unique", SearchFilter{})
	if err != nil {
		t.Fatalf("Search after update: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("after update: got %d, want 1", len(results))
	}

	// The old title should not match anymore.
	results, err = s.Search(ctx, "original", SearchFilter{})
	if err != nil {
		t.Fatalf("Search old title: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("old title still matches after update, got %d results", len(results))
	}

	// After delete — zero results.
	if err := s.Delete(ctx, tk.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	results, err = s.Search(ctx, "unique", SearchFilter{})
	if err != nil {
		t.Fatalf("Search after delete: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("deleted task still in FTS, got %d results", len(results))
	}
}
