package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tui.db")
	s, err := store.OpenSQLite(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return New(s, nil)
}

func seedTasks(t *testing.T, m Model) {
	t.Helper()
	ctx := context.Background()
	m.store.Add(ctx, store.AddInput{Title: "task one", Priority: task.PriorityHigh, Ready: true})
	m.store.Add(ctx, store.AddInput{Title: "task two", Ready: true})
	m.store.Add(ctx, store.AddInput{Title: "task three", Ready: true})
}

// apply runs Update synchronously, and if it returns a Cmd — executes it and recurses on the result.
// Handles BatchMsg (tea.Batch) by expanding into individual Cmds recursively.
func apply(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	// Expand BatchMsg.
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if inner := c(); inner != nil {
				m = apply(t, m, inner)
			}
		}
		return m
	}
	newM, cmd := m.Update(msg)
	m = newM.(Model)
	if cmd != nil {
		if result := cmd(); result != nil {
			m = apply(t, m, result)
		}
	}
	return m
}

func key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func special(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func TestInitLoadsTasks(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)

	m = apply(t, m, m.Init()())

	if len(m.tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(m.tasks))
	}
}

func TestCursorNavigation(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	m = apply(t, m, key('j'))
	if m.cursor != 1 {
		t.Errorf("cursor = %d after j, want 1", m.cursor)
	}

	m = apply(t, m, key('k'))
	if m.cursor != 0 {
		t.Errorf("cursor = %d after k, want 0", m.cursor)
	}

	// k at zero should not go to -1
	m = apply(t, m, key('k'))
	if m.cursor != 0 {
		t.Errorf("cursor = %d after k at 0, want 0", m.cursor)
	}
}

func TestTabSwitching(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	if m.tab != tabActive {
		t.Fatalf("initial tab = %v, want active", m.tab)
	}

	m = apply(t, m, special(tea.KeyTab))
	if m.tab != tabDone {
		t.Errorf("tab = %v after tab, want done", m.tab)
	}
}

func TestDetailToggle(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeDetail {
		t.Error("expected detail mode after enter")
	}

	m = apply(t, m, special(tea.KeyEscape))
	if m.mode != modeList {
		t.Error("expected list mode after esc")
	}
}

func TestCycleStatus(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// t: todo → doing
	m = apply(t, m, key('t'))
	if got := findStatus(m, "task one"); got != task.StatusDoing {
		t.Errorf("after 1× t: task one status = %v, want doing", got)
	}

	// t: doing → action
	m = apply(t, m, key('t'))
	if got := findStatus(m, "task one"); got != task.StatusAction {
		t.Errorf("after 2× t: task one status = %v, want action", got)
	}

	// t: action → todo (closes the cycle, does not advance to done)
	m = apply(t, m, key('t'))
	if got := findStatus(m, "task one"); got != task.StatusTodo {
		t.Errorf("after 3× t: task one status = %v, want todo", got)
	}
}

func TestMarkDoneKey(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// x moves todo → done (disappears from the active view)
	m = apply(t, m, key('x'))
	for _, tk := range m.tasks {
		if tk.Title == "task one" {
			t.Errorf("task one should not be in active view after x, status=%v", tk.Status)
		}
	}

	// Switching to the done tab confirms it moved there.
	m = apply(t, m, special(tea.KeyRight))
	found := false
	for _, tk := range m.tasks {
		if tk.Title == "task one" && tk.Status == task.StatusDone {
			found = true
		}
	}
	if !found {
		t.Error("task one should appear as done on done tab")
	}
}

func findStatus(m Model, title string) task.Status {
	for _, tk := range m.tasks {
		if tk.Title == title {
			return tk.Status
		}
	}
	return ""
}

func TestViewRenders(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())
	m.width = 80
	m.height = 24

	view := m.View()
	if !strings.Contains(view, "task one") {
		t.Errorf("view should contain 'task one', got:\n%s", view)
	}
	if !strings.Contains(view, "active") {
		t.Errorf("view should contain 'active' tab, got:\n%s", view)
	}
}

func TestQuitMessage(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(key('q'))
	if cmd == nil {
		t.Fatal("q should produce quit cmd")
	}
}

func TestSearchMode(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	m.store.Add(ctx, store.AddInput{Title: "deploy backend", Ready: true})
	m.store.Add(ctx, store.AddInput{Title: "fix frontend", Ready: true})
	m = apply(t, m, m.Init()())

	// / → tryb search
	m = apply(t, m, key('/'))
	if m.mode != modeSearch {
		t.Fatalf("mode = %v, want search", m.mode)
	}

	// Wpisujemy "backend" — live filtering
	for _, r := range "backend" {
		m = apply(t, m, key(r))
	}
	if len(m.tasks) != 1 {
		t.Errorf("got %d tasks during search, want 1", len(m.tasks))
	}

	// Enter potwierdza
	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeList {
		t.Errorf("mode = %v after enter, want list", m.mode)
	}
	if m.searchQuery != "backend" {
		t.Errorf("searchQuery = %q, want 'backend'", m.searchQuery)
	}

	// Esc clears search
	m = apply(t, m, key('/'))
	m = apply(t, m, special(tea.KeyEscape))
	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q after esc, want empty", m.searchQuery)
	}
}

func TestAddMode(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())

	// a → add mode
	m = apply(t, m, key('a'))
	if m.mode != modeAdd {
		t.Fatalf("mode = %v, want add", m.mode)
	}

	// Type the title
	for _, r := range "new task" {
		m = apply(t, m, key(r))
	}
	if m.input != "new task" {
		t.Errorf("input = %q, want 'new task'", m.input)
	}

	// Enter → second step (tags), prefill empty because no context/filterTags
	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeAddTags {
		t.Fatalf("mode = %v after first enter, want addTags", m.mode)
	}
	if m.pendingAddTitle != "new task" {
		t.Errorf("pendingAddTitle = %q, want 'new task'", m.pendingAddTitle)
	}
	if m.input != "" {
		t.Errorf("tags prefill = %q, want empty", m.input)
	}

	// Enter → adds without tags
	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeList {
		t.Errorf("mode = %v after tag enter, want list", m.mode)
	}
	if m.pendingAddTitle != "" {
		t.Errorf("pendingAddTitle = %q after commit, want empty", m.pendingAddTitle)
	}

	// Weryfikujemy w store
	tasks, _ := m.store.List(context.Background(), store.ListFilter{})
	found := false
	for _, tk := range tasks {
		if tk.Title == "new task" {
			found = true
		}
	}
	if !found {
		t.Error("'new task' not found in store after add")
	}
}

func TestAddModePrefillsContextAsTag(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())
	// Symulujemy aktywny kontekst (jak po C).
	m.context = "work"

	m = apply(t, m, key('a'))
	for _, r := range "ctx task" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeAddTags {
		t.Fatalf("mode = %v, want addTags", m.mode)
	}
	if m.input != "work" {
		t.Errorf("tags prefill = %q, want 'work'", m.input)
	}

	// Enter → zapisujemy z tagiem 'work'.
	m = apply(t, m, special(tea.KeyEnter))
	tasks, _ := m.store.List(context.Background(), store.ListFilter{Tags: []string{"work"}})
	var found *task.Task
	for i, tk := range tasks {
		if tk.Title == "ctx task" {
			found = &tasks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("task 'ctx task' with tag 'work' not found")
	}
	hasTag := false
	for _, tg := range found.Tags {
		if tg == "work" {
			hasTag = true
		}
	}
	if !hasTag {
		t.Errorf("tags = %v, want contain 'work'", found.Tags)
	}
}

func TestAddModeCancelAtTagsStep(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())

	m = apply(t, m, key('a'))
	for _, r := range "will cancel" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))
	if m.mode != modeAddTags {
		t.Fatalf("mode = %v, want addTags", m.mode)
	}
	// Esc cancels the whole thing — task should not be created.
	m = apply(t, m, special(tea.KeyEscape))
	if m.mode != modeList {
		t.Errorf("mode = %v after esc, want list", m.mode)
	}
	if m.pendingAddTitle != "" {
		t.Errorf("pendingAddTitle = %q after esc, want empty", m.pendingAddTitle)
	}
	tasks, _ := m.store.List(context.Background(), store.ListFilter{})
	for _, tk := range tasks {
		if tk.Title == "will cancel" {
			t.Errorf("task 'will cancel' was created but should have been cancelled")
		}
	}
}

func TestEditMode(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// e → edit mode with prefill
	m = apply(t, m, key('e'))
	if m.mode != modeEdit {
		t.Fatalf("mode = %v, want edit", m.mode)
	}
	if m.input != "task one" {
		t.Errorf("input = %q, want pre-filled 'task one'", m.input)
	}

	// Clear and type the new title
	m = apply(t, m, special(tea.KeyCtrlU))
	for _, r := range "edited task" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))

	// Weryfikujemy w store
	tk, err := m.store.Get(context.Background(), m.tasks[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tk.Title != "edited task" {
		t.Errorf("title = %q, want 'edited task'", tk.Title)
	}
}

func TestDeleteConfirm(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())
	initialCount := len(m.tasks)

	// d → confirm
	m = apply(t, m, key('d'))
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want confirm", m.mode)
	}
	if !strings.Contains(m.confirmMsg, "task one") {
		t.Errorf("confirmMsg = %q, want mention of 'task one'", m.confirmMsg)
	}

	// y → usuwamy
	m = apply(t, m, key('y'))
	if m.mode != modeList {
		t.Errorf("mode = %v after y, want list", m.mode)
	}
	if len(m.tasks) != initialCount-1 {
		t.Errorf("got %d tasks after delete, want %d", len(m.tasks), initialCount-1)
	}
}

func TestDeleteCancel(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())
	initialCount := len(m.tasks)

	m = apply(t, m, key('d'))
	m = apply(t, m, key('n'))
	if m.mode != modeList {
		t.Errorf("mode = %v after n, want list", m.mode)
	}
	if len(m.tasks) != initialCount {
		t.Errorf("got %d tasks after cancel, want %d", len(m.tasks), initialCount)
	}
}

func TestPublish(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	// Add a draft (Ready=false by default)
	m.store.Add(ctx, store.AddInput{Title: "draft task"})
	m = apply(t, m, m.Init()())

	// Verify it's a draft
	if !m.tasks[0].Draft {
		t.Fatal("task should be draft")
	}

	// p → publish
	m = apply(t, m, key('p'))
	if m.tasks[0].Draft {
		t.Error("task should not be draft after publish")
	}
}

func TestSetPriority(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// task one (id=1) = high, is first on the list (cursor=0).
	taskID := m.tasks[m.cursor].ID

	// Change to low (3).
	m = apply(t, m, key('3'))
	tk, err := m.store.Get(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tk.Priority != task.PriorityLow {
		t.Errorf("priority = %q, want low", tk.Priority)
	}

	// Change to normal (2) — the task may have moved, look it up by ID.
	tk, _ = m.store.Get(context.Background(), taskID)
	// Move cursor onto the right task.
	for i, t := range m.tasks {
		if t.ID == taskID {
			m.cursor = i
			break
		}
	}
	m = apply(t, m, key('2'))
	tk, _ = m.store.Get(context.Background(), taskID)
	if tk.Priority != task.PriorityNormal {
		t.Errorf("priority = %q, want normal", tk.Priority)
	}

	for i, t := range m.tasks {
		if t.ID == taskID {
			m.cursor = i
			break
		}
	}
	m = apply(t, m, key('1'))
	tk, _ = m.store.Get(context.Background(), taskID)
	if tk.Priority != task.PriorityHigh {
		t.Errorf("priority = %q, want high", tk.Priority)
	}
}

func TestSetPriorityFromDetail(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	taskID := m.tasks[m.cursor].ID
	m = apply(t, m, special(tea.KeyEnter)) // detail
	m = apply(t, m, key('3'))              // low

	tk, _ := m.store.Get(context.Background(), taskID)
	if tk.Priority != task.PriorityLow {
		t.Errorf("priority = %q, want low (from detail)", tk.Priority)
	}
}

func TestEscClearsSearchFromList(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	m.store.Add(ctx, store.AddInput{Title: "deploy backend", Ready: true})
	m.store.Add(ctx, store.AddInput{Title: "fix frontend", Ready: true})
	m = apply(t, m, m.Init()())

	// Szukamy i potwierdzamy enter
	m = apply(t, m, key('/'))
	for _, r := range "backend" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))
	if m.searchQuery != "backend" {
		t.Fatalf("searchQuery = %q, want 'backend'", m.searchQuery)
	}
	if len(m.tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(m.tasks))
	}

	// Esc on the list clears the search
	m = apply(t, m, special(tea.KeyEscape))
	if m.searchQuery != "" {
		t.Errorf("searchQuery = %q after esc, want empty", m.searchQuery)
	}
	if len(m.tasks) != 2 {
		t.Errorf("got %d tasks after clear, want 2", len(m.tasks))
	}
}

func TestAddWithSpaces(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())

	m = apply(t, m, key('a'))
	for _, r := range "hello" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeySpace))
	for _, r := range "world" {
		m = apply(t, m, key(r))
	}
	if m.input != "hello world" {
		t.Errorf("input = %q, want 'hello world'", m.input)
	}
	// Enter → tag step, another Enter → save without tags.
	m = apply(t, m, special(tea.KeyEnter))
	m = apply(t, m, special(tea.KeyEnter))

	tasks, _ := m.store.List(context.Background(), store.ListFilter{})
	found := false
	for _, tk := range tasks {
		if tk.Title == "hello world" {
			found = true
		}
	}
	if !found {
		t.Error("'hello world' not found — space not handled in input")
	}
}

func TestBodyEdit(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// b → body edit
	m = apply(t, m, key('b'))
	if m.mode != modeBody {
		t.Fatalf("mode = %v, want body", m.mode)
	}

	// Type body with Enter (newline)
	for _, r := range "line1" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))
	for _, r := range "line2" {
		m = apply(t, m, key(r))
	}
	if m.input != "line1\nline2" {
		t.Errorf("input = %q, want 'line1\\nline2'", m.input)
	}

	// ctrl+s zapisuje
	m = apply(t, m, special(tea.KeyCtrlS))
	if m.mode != modeList {
		t.Errorf("mode = %v after ctrl+s, want list", m.mode)
	}

	// Weryfikujemy w store
	tk, err := m.store.Get(context.Background(), m.tasks[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tk.Body != "line1\nline2" {
		t.Errorf("body = %q, want 'line1\\nline2'", tk.Body)
	}
}

func TestBodyEditCancel(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	m = apply(t, m, key('b'))
	for _, r := range "unsaved" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEscape))
	if m.mode != modeList {
		t.Errorf("mode = %v after esc, want list", m.mode)
	}
	// Body should not change.
	tk, _ := m.store.Get(context.Background(), m.tasks[0].ID)
	if tk.Body != "" {
		t.Errorf("body = %q after cancel, want empty", tk.Body)
	}
}

func TestBodyEditFromDetail(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// enter → detail, b → body
	m = apply(t, m, special(tea.KeyEnter))
	m = apply(t, m, key('b'))
	if m.mode != modeBody {
		t.Fatalf("mode = %v, want body from detail", m.mode)
	}
}

func TestAddEmptyTitleCancels(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())

	m = apply(t, m, key('a'))
	m = apply(t, m, special(tea.KeyEnter)) // empty
	if m.mode != modeList {
		t.Errorf("mode = %v, want list (empty add should cancel)", m.mode)
	}
}

func TestInputBackspace(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())
	m = apply(t, m, key('a'))

	for _, r := range "abc" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyBackspace))
	if m.input != "ab" {
		t.Errorf("input = %q after backspace, want 'ab'", m.input)
	}
}

func TestWorklogTabCycle(t *testing.T) {
	m := newTestModel(t)
	seedTasks(t, m)
	m = apply(t, m, m.Init()())

	// active → done → worklog → active
	m = apply(t, m, special(tea.KeyTab))
	if m.tab != tabDone {
		t.Fatalf("tab = %v after 1×tab, want done", m.tab)
	}
	m = apply(t, m, special(tea.KeyTab))
	if m.tab != tabWorklog {
		t.Fatalf("tab = %v after 2×tab, want worklog", m.tab)
	}
	m = apply(t, m, special(tea.KeyTab))
	if m.tab != tabActive {
		t.Fatalf("tab = %v after 3×tab, want active (wrap)", m.tab)
	}
}

func TestWorklogLoadsEntries(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	tk, err := m.store.Add(ctx, store.AddInput{Title: "alpha", Ready: true})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	base := time.Now().Add(-2 * time.Hour)
	if _, err := m.store.CreateTimeEntry(ctx, store.CreateTimeEntryInput{
		TaskID: tk.ID, Start: base, End: base.Add(30 * time.Minute), Note: "first",
	}); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	m = apply(t, m, m.Init()())

	// Switch to worklog (2 tabs: active → done → worklog)
	m = apply(t, m, special(tea.KeyTab))
	m = apply(t, m, special(tea.KeyTab))

	if len(m.worklogEntries) != 1 {
		t.Fatalf("worklogEntries = %d, want 1", len(m.worklogEntries))
	}
	if m.worklogEntries[0].TaskTitle != "alpha" {
		t.Errorf("TaskTitle = %q, want 'alpha'", m.worklogEntries[0].TaskTitle)
	}
}

func TestWorklogRangeDefault(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())
	m.tab = tabWorklog
	if m.worklogRange != wrWeek {
		t.Fatalf("initial range = %v, want week", m.worklogRange)
	}
}

func TestWorklogRangeCustom(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())
	m.tab = tabWorklog

	// 'd' otwiera input date-range
	m = apply(t, m, key('d'))
	if m.mode != modeWorklogRange {
		t.Fatalf("after d: mode = %v, want modeWorklogRange", m.mode)
	}
	if m.worklogRangeField != 0 {
		t.Errorf("rangeField = %d, want 0 (from)", m.worklogRangeField)
	}
}

func TestWorklogViewRenders(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	tk, _ := m.store.Add(ctx, store.AddInput{Title: "render me", Ready: true})
	base := time.Now().Add(-2 * time.Hour)
	m.store.CreateTimeEntry(ctx, store.CreateTimeEntryInput{
		TaskID: tk.ID, Start: base, End: base.Add(time.Hour), Note: "hello",
	})
	m = apply(t, m, m.Init()())
	m = apply(t, m, special(tea.KeyTab))
	m = apply(t, m, special(tea.KeyTab))
	m.width = 120
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "worklog") {
		t.Errorf("view missing 'worklog' tab label:\n%s", view)
	}
	if !strings.Contains(view, "render me") {
		t.Errorf("view missing task title:\n%s", view)
	}
	if !strings.Contains(view, "1 session") {
		t.Errorf("view missing session count:\n%s", view)
	}
	if !strings.Contains(view, "total:") {
		t.Errorf("view missing total footer:\n%s", view)
	}
}

func TestWorklogEmptyMessage(t *testing.T) {
	m := newTestModel(t)
	m = apply(t, m, m.Init()())
	m = apply(t, m, special(tea.KeyTab))
	m = apply(t, m, special(tea.KeyTab))
	m.width = 80
	m.height = 24

	view := m.View()
	if !strings.Contains(view, "no time entries") {
		t.Errorf("empty worklog should show 'no time entries':\n%s", view)
	}
}

func TestSearchViewShowsQuery(t *testing.T) {
	m := newTestModel(t)
	ctx := context.Background()
	m.store.Add(ctx, store.AddInput{Title: "searchable item", Ready: true})
	m = apply(t, m, m.Init()())

	// Ustawiamy search query i wracamy do list
	m = apply(t, m, key('/'))
	for _, r := range "search" {
		m = apply(t, m, key(r))
	}
	m = apply(t, m, special(tea.KeyEnter))

	view := m.View()
	if !strings.Contains(view, "/search") {
		t.Errorf("view should show active search query, got:\n%s", view)
	}
}
