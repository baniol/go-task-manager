package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"go-task-manager/internal/config"
	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
	"go-task-manager/internal/timefmt"
)

// --- style ---

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	helpStyle    = lipgloss.NewStyle().Faint(true)
	selStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	draftStyle   = lipgloss.NewStyle().Faint(true)
	dueWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusTodo   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	statusDo     = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusAction = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	statusDone   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	prioHigh     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	prioLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	detailKey    = lipgloss.NewStyle().Bold(true).Width(10)
	inputStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	promptStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	timerActive  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	timerHas     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// --- messages ---

type tasksMsg []task.Task
type errMsg struct{ err error }
type flashMsg string
type mdRendererMsg struct{ r *glamour.TermRenderer }
type editorFinishedMsg struct {
	taskID  int64
	content string
	err     error
}

// activeTimerMsg — current active entry (nil = none). Title is included for the header.
type activeTimerMsg struct {
	entry *task.TimeEntry
	title string
}

// detailLogMsg — time-entry log for the currently opened task (right column in detail).
type detailLogMsg struct {
	taskID  int64
	entries []task.TimeEntry
}

// hasEntriesMsg — set of task IDs that have log entries (for the list icon).
type hasEntriesMsg map[int64]bool

// worklogMsg — entries for the worklog tab.
type worklogMsg []store.WorklogEntry

// tickMsg — once-per-second signal while a timer is active (refreshes the header).
type tickMsg time.Time

func (e errMsg) Error() string { return e.err.Error() }

// --- interaction mode ---

type mode int

const (
	modeList          mode = iota // normal navigation
	modeDetail                    // detail view
	modeSearch                    // typing a search/filter query
	modeAdd                       // typing a new task title
	modeAddTags                   // second step after modeAdd — typing tags for the new task
	modeEdit                      // editing task title
	modeConfirm                   // confirmation prompt (e.g. delete)
	modeTagAdd                    // adding a tag to a task
	modeTagRm                     // removing a tag from a task
	modeTagFilter                 // filtering by tag
	modeLogEdit                   // sequential time-entry edit (start→end→note)
	modeWorklogRange              // typing a date range for worklog (from→to)
	modeContextPicker             // picking the active context (tag)
)

// --- tab (status filter) ---

type tab int

const (
	tabActive  tab = iota // todo + doing
	tabDone               // done
	tabWorklog            // time-entries view
)

const tabCount = 3

func (t tab) String() string {
	switch t {
	case tabDone:
		return "done"
	case tabWorklog:
		return "worklog"
	default:
		return "active"
	}
}

func (t tab) statuses() []task.Status {
	switch t {
	case tabDone:
		return []task.Status{task.StatusDone}
	default:
		return []task.Status{task.StatusTodo, task.StatusDoing, task.StatusAction}
	}
}

// --- worklog range (time filter in the TUI) ---

type worklogRange int

const (
	wrWeek  worklogRange = iota // current ISO week (default)
	wrRange                     // user-supplied date range
)

// resolveWorklogRange returns [from, to) for the given range.
func resolveWorklogRange(r worklogRange, customFrom, customTo *time.Time, now time.Time) (*time.Time, *time.Time) {
	switch r {
	case wrRange:
		return customFrom, customTo
	default: // wrWeek
		d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		offset := int(d.Weekday()) - 1
		if offset < 0 {
			offset = 6
		}
		start := d.AddDate(0, 0, -offset)
		end := start.AddDate(0, 0, 7)
		return &start, &end
	}
}

// --- model ---

type Model struct {
	store    store.Store
	tasks    []task.Task
	cursor   int
	tab      tab
	mode     mode
	width    int
	height   int
	err      error
	quitting bool

	// input — shared buffer for search/add/edit
	input       string
	inputCursor int

	// pendingAddTitle — new task title remembered between modeAdd → modeAddTags
	pendingAddTitle string

	// search — active text filter
	searchQuery string
	// filterTags — active tag filter
	filterTags []string

	// confirm — context for confirmation prompt (delete)
	confirmMsg    string
	confirmAction func() tea.Msg

	// flash — short feedback message
	flash string

	// active — active timer (nil = none)
	active      *task.TimeEntry
	activeTitle string
	// now — refreshed by tick, used to render elapsed time
	now time.Time

	// detailLog — time entries for the currently opened task (detail mode)
	detailLog       []task.TimeEntry
	detailLogTaskID int64
	// detailFocus: 0 = left column (metadata), 1 = right (log)
	detailFocus int
	// detailLogCursor — index of the selected entry in detailLog (view: newest on top)
	detailLogCursor int

	// logEdit — state of sequential entry edit
	logEditEntryID int64
	logEditField   int // 0=date, 1=start, 2=end, 3=note
	logEditStart   time.Time
	logEditEnd     time.Time
	logEditNote    string

	// hasEntries — task IDs with at least one time entry (list icon)
	hasEntries map[int64]bool

	// worklog — state for the worklog tab (cross-task entries)
	worklogEntries    []store.WorklogEntry
	worklogRange      worklogRange
	worklogCustomFrom *time.Time
	worklogCustomTo   *time.Time
	worklogRangeField int // 0=from, 1=to (during modeWorklogRange)

	// context — active context (tag); "" = no filter
	context string
	// cfg — config file
	cfg *config.Config
	// contextTags — full tag list for the picker
	contextTags   []string
	contextCursor int
	contextSearch string

	// mdRenderer — glamour renderer for body markdown preview (created once in New)
	mdRenderer *glamour.TermRenderer
}

func New(s store.Store, cfg *config.Config) Model {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return Model{
		store:   s,
		width:   80,
		height:  24,
		now:     time.Now(),
		cfg:     cfg,
		context: cfg.Context,
	}
}

func initMdRenderer(wordWrap int) tea.Cmd {
	return func() tea.Msg {
		r, _ := glamour.NewTermRenderer(glamour.WithAutoStyle(), glamour.WithWordWrap(wordWrap))
		return mdRendererMsg{r: r}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchTasks(), m.fetchActiveTimer(), m.fetchHasEntries(), initMdRenderer(60))
}

func (m Model) fetchHasEntries() tea.Cmd {
	s := m.store
	return func() tea.Msg {
		set, err := s.TasksWithTimeEntries(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return hasEntriesMsg(set)
	}
}

func (m Model) fetchDetailLog(taskID int64) tea.Cmd {
	s := m.store
	return func() tea.Msg {
		entries, err := s.TimeEntries(context.Background(), taskID)
		if err != nil {
			return errMsg{err}
		}
		return detailLogMsg{taskID: taskID, entries: entries}
	}
}

func (m Model) fetchActiveTimer() tea.Cmd {
	s := m.store
	return func() tea.Msg {
		ctx := context.Background()
		active, err := s.ActiveTimer(ctx)
		if err != nil {
			return errMsg{err}
		}
		var title string
		if active != nil {
			if t, err := s.Get(ctx, active.TaskID); err == nil {
				title = t.Title
			}
		}
		return activeTimerMsg{entry: active, title: title}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetchForTab picks the right fetch based on the active tab.
func (m Model) fetchForTab() tea.Cmd {
	if m.tab == tabWorklog {
		return m.fetchWorklog()
	}
	return m.fetchTasks()
}

func (m Model) fetchWorklog() tea.Cmd {
	s := m.store
	from, to := resolveWorklogRange(m.worklogRange, m.worklogCustomFrom, m.worklogCustomTo, time.Now())
	tags := mergeContext(m.context, m.filterTags)
	search := m.searchQuery
	return func() tea.Msg {
		filter := store.TimeEntryFilter{
			From: from, To: to, Tags: tags, Search: search, Limit: 500,
		}
		entries, err := s.AllTimeEntries(context.Background(), filter)
		if err != nil {
			return errMsg{err}
		}
		return worklogMsg(entries)
	}
}

func (m Model) fetchTasks() tea.Cmd {
	s := m.store
	t := m.tab
	q := m.searchQuery
	tags := mergeContext(m.context, m.filterTags)
	return func() tea.Msg {
		ctx := context.Background()
		if q != "" {
			tasks, err := s.Search(ctx, q, store.SearchFilter{Statuses: t.statuses(), Tags: tags})
			if err != nil {
				return errMsg{err}
			}
			return tasksMsg(tasks)
		}
		filter := store.ListFilter{Statuses: t.statuses(), Sort: store.SortPosition, Tags: tags}
		tasks, err := s.List(ctx, filter)
		if err != nil {
			return errMsg{err}
		}
		return tasksMsg(tasks)
	}
}

// fetchTaskList — shared helper to refresh the list after a mutation.
func fetchTaskList(ctx context.Context, s store.Store, sq string, t tab, tags []string) tea.Msg {
	if sq != "" {
		tasks, err := s.Search(ctx, sq, store.SearchFilter{Statuses: t.statuses(), Tags: tags})
		if err != nil {
			return errMsg{err}
		}
		return tasksMsg(tasks)
	}
	tasks, err := s.List(ctx, store.ListFilter{Statuses: t.statuses(), Sort: store.SortPosition, Tags: tags})
	if err != nil {
		return errMsg{err}
	}
	return tasksMsg(tasks)
}

// mergeContext combines the active context with extra tag filters (AND).
func mergeContext(ctx string, filterTags []string) []string {
	if ctx == "" {
		return filterTags
	}
	for _, t := range filterTags {
		if t == ctx {
			return filterTags
		}
	}
	return append([]string{ctx}, filterTags...)
}

// --- update ---

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		rightW := min(max(m.width/2, 42), 70)
		leftW := max(m.width-rightW-4, 20)
		return m, initMdRenderer(max(leftW-2, 20))

	case mdRendererMsg:
		m.mdRenderer = msg.r
		return m, nil

	case tasksMsg:
		m.tasks = msg
		m.err = nil
		if m.cursor >= len(m.tasks) {
			m.cursor = max(0, len(m.tasks)-1)
		}
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case flashMsg:
		m.flash = string(msg)
		return m, nil

	case activeTimerMsg:
		wasActive := m.active != nil
		m.active = msg.entry
		m.activeTitle = msg.title
		// Refresh "has entries" set (start adds; stop doesn't change it, but cheaper than branching).
		cmds := []tea.Cmd{m.fetchHasEntries()}
		if m.mode == modeDetail && len(m.tasks) > 0 {
			cmds = append(cmds, m.fetchDetailLog(m.tasks[m.cursor].ID))
		}
		if m.tab == tabWorklog {
			cmds = append(cmds, m.fetchWorklog())
		}
		if m.active != nil && !wasActive {
			m.now = time.Now()
			cmds = append(cmds, tickCmd())
		}
		return m, tea.Batch(cmds...)

	case detailLogMsg:
		m.detailLog = msg.entries
		m.detailLogTaskID = msg.taskID
		if m.detailLogCursor >= len(m.detailLog) {
			m.detailLogCursor = max(0, len(m.detailLog)-1)
		}
		// Log may have changed via delete/edit — refresh the "has entries" set.
		return m, m.fetchHasEntries()

	case hasEntriesMsg:
		m.hasEntries = map[int64]bool(msg)
		return m, nil

	case worklogMsg:
		m.worklogEntries = msg
		m.err = nil
		if m.cursor >= len(m.worklogEntries) {
			m.cursor = max(0, len(m.worklogEntries)-1)
		}
		return m, nil

	case contextTagsMsg:
		m.contextTags = []string(msg)
		m.contextSearch = ""
		m.contextCursor = 0
		filtered := m.filteredContextTags()
		for i, tag := range filtered {
			if tag == m.context {
				m.contextCursor = i
				break
			}
		}
		if m.context == "" {
			m.contextCursor = len(filtered)
		}
		m.mode = modeContextPicker
		return m, nil

	case editorFinishedMsg:
		slog.Debug("editorFinishedMsg received", "taskID", msg.taskID, "hasErr", msg.err != nil, "mode", m.mode)
		if msg.err != nil {
			return m, nil // editor exited non-zero (e.g. :cq) — treat as cancel
		}
		body := msg.content
		taskID := msg.taskID
		s := m.store
		t := m.tab
		sq := m.searchQuery
		tags := mergeContext(m.context, m.filterTags)
		return m, func() tea.Msg {
			ctx := context.Background()
			slog.Debug("editorFinishedMsg: Update begin", "taskID", taskID)
			if err := s.Update(ctx, taskID, store.EditInput{Body: &body}); err != nil {
				slog.Error("editorFinishedMsg: Update failed", "err", err)
				return errMsg{err}
			}
			slog.Debug("editorFinishedMsg: Update done, fetching list")
			msg := fetchTaskList(ctx, s, sq, t, tags)
			slog.Debug("editorFinishedMsg: fetch done")
			return msg
		}

	case tickMsg:
		m.now = time.Time(msg)
		if m.active == nil {
			return m, nil
		}
		return m, tickCmd()

	case tea.KeyMsg:
		// Global: ctrl+c → quit.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		// Clear flash on any keypress.
		m.flash = ""

		switch m.mode {
		case modeSearch:
			return m.updateInput(msg, m.commitSearch, m.cancelSearch)
		case modeAdd:
			return m.updateInput(msg, m.commitAdd, m.cancelInput)
		case modeAddTags:
			return m.updateInput(msg, m.commitAddTags, m.cancelAddTags)
		case modeEdit:
			return m.updateInput(msg, m.commitEdit, m.cancelInput)
		case modeTagAdd:
			return m.updateInput(msg, m.commitTagAdd, m.cancelInput)
		case modeTagRm:
			return m.updateInput(msg, m.commitTagRm, m.cancelInput)
		case modeTagFilter:
			return m.updateInput(msg, m.commitTagFilter, m.cancelInput)
		case modeLogEdit:
			return m.updateInput(msg, m.commitLogEditField, m.cancelLogEdit)
		case modeWorklogRange:
			return m.updateInput(msg, m.commitWorklogRangeField, m.cancelWorklogRange)
		case modeContextPicker:
			return m.updateContextPicker(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		case modeDetail:
			return m.updateDetail(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

// --- input mode (search / add / edit) ---

func (m *Model) startInput(mode mode, initial string) {
	m.mode = mode
	m.input = initial
	m.inputCursor = len(initial)
}

func (m Model) insertChar(ch string) Model {
	m.input = m.input[:m.inputCursor] + ch + m.input[m.inputCursor:]
	m.inputCursor += len(ch)
	return m
}

// handleTextEdit — shared handling for text-edit keys (backspace, navigation, insert).
func (m Model) handleTextEdit(msg tea.KeyMsg) (Model, bool) {
	switch msg.Type {
	case tea.KeyBackspace:
		if m.inputCursor > 0 {
			m.input = m.input[:m.inputCursor-1] + m.input[m.inputCursor:]
			m.inputCursor--
		}
	case tea.KeyLeft:
		if m.inputCursor > 0 {
			m.inputCursor--
		}
	case tea.KeyRight:
		if m.inputCursor < len(m.input) {
			m.inputCursor++
		}
	case tea.KeyCtrlA, tea.KeyHome:
		m.inputCursor = 0
	case tea.KeyCtrlE, tea.KeyEnd:
		m.inputCursor = len(m.input)
	case tea.KeyCtrlU:
		m.input = m.input[m.inputCursor:]
		m.inputCursor = 0
	case tea.KeySpace:
		m = m.insertChar(" ")
	case tea.KeyRunes:
		m = m.insertChar(string(msg.Runes))
	default:
		return m, false
	}
	return m, true
}

func (m Model) updateInput(msg tea.KeyMsg, commit func(Model) (Model, tea.Cmd), cancel func(Model) (Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return commit(m)
	case tea.KeyEscape:
		return cancel(m)
	default:
		m, _ = m.handleTextEdit(msg)
	}
	// Live search — update results as the user types.
	if m.mode == modeSearch {
		m.searchQuery = m.input
		return m, m.fetchForTab()
	}
	return m, nil
}

// launchBodyEditor opens $EDITOR (fallback: vi) with the task body in a temp file.
// On return, editorFinishedMsg is dispatched with the saved content.
func launchBodyEditor(taskID int64, body string) tea.Cmd {
	f, err := os.CreateTemp("", "tm-body-*.md")
	if err != nil {
		slog.Error("editor: CreateTemp failed", "err", err)
		return func() tea.Msg { return errMsg{err} }
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		os.Remove(f.Name())
		slog.Error("editor: write temp failed", "err", err)
		return func() tea.Msg { return errMsg{err} }
	}
	f.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	name := f.Name()
	slog.Debug("editor: launching", "editor", editor, "tmp", name, "taskID", taskID, "bodyLen", len(body))
	c := exec.Command(editor, name)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		slog.Debug("editor: ExecProcess returned", "err", err)
		content, readErr := os.ReadFile(name)
		os.Remove(name)
		if readErr != nil {
			slog.Error("editor: read temp failed", "err", readErr)
			return errMsg{readErr}
		}
		// Non-zero exit (e.g. :cq in vim) is treated as cancel — no save.
		if err != nil {
			slog.Debug("editor: non-zero exit, treating as cancel", "err", err)
			return editorFinishedMsg{taskID: taskID, content: body, err: err}
		}
		slog.Debug("editor: sending editorFinishedMsg", "taskID", taskID, "contentLen", len(content))
		return editorFinishedMsg{taskID: taskID, content: string(content)}
	})
}

func (m Model) commitSearch(mod Model) (Model, tea.Cmd) {
	mod.searchQuery = strings.TrimSpace(mod.input)
	mod.mode = modeList
	mod.cursor = 0
	return mod, mod.fetchForTab()
}

func (m Model) cancelSearch(mod Model) (Model, tea.Cmd) {
	mod.searchQuery = ""
	mod.input = ""
	mod.mode = modeList
	mod.cursor = 0
	return mod, mod.fetchForTab()
}

func (m Model) cancelInput(mod Model) (Model, tea.Cmd) {
	mod.mode = modeList
	mod.input = ""
	return mod, nil
}

func (m Model) commitAdd(mod Model) (Model, tea.Cmd) {
	title := strings.TrimSpace(mod.input)
	if title == "" {
		mod.mode = modeList
		mod.input = ""
		mod.pendingAddTitle = ""
		return mod, nil
	}
	// After the title — second step: tags, pre-filled with active filter (context + filterTags).
	mod.pendingAddTitle = title
	prefill := strings.Join(mergeContext(mod.context, mod.filterTags), ", ")
	mod.startInput(modeAddTags, prefill)
	return mod, nil
}

func (m Model) commitAddTags(mod Model) (Model, tea.Cmd) {
	title := mod.pendingAddTitle
	if title == "" {
		// Safety: no title — bail back to list without changes.
		mod.mode = modeList
		mod.input = ""
		mod.pendingAddTitle = ""
		return mod, nil
	}
	names := splitTagInput(mod.input)
	mod.mode = modeList
	mod.input = ""
	mod.pendingAddTitle = ""
	s := mod.store
	t := mod.tab
	sq := mod.searchQuery
	tags := mergeContext(mod.context, mod.filterTags)
	return mod, func() tea.Msg {
		ctx := context.Background()
		if _, err := s.Add(ctx, store.AddInput{Title: title, Tags: names, Ready: true}); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

func (m Model) cancelAddTags(mod Model) (Model, tea.Cmd) {
	mod.mode = modeList
	mod.input = ""
	mod.pendingAddTitle = ""
	return mod, nil
}

func (m Model) commitEdit(mod Model) (Model, tea.Cmd) {
	title := strings.TrimSpace(mod.input)
	if title == "" || len(mod.tasks) == 0 {
		mod.mode = modeList
		mod.input = ""
		return mod, nil
	}
	taskID := mod.tasks[mod.cursor].ID
	mod.mode = modeList
	mod.input = ""
	s := mod.store
	t := mod.tab
	sq := mod.searchQuery
	tags := mergeContext(mod.context, mod.filterTags)
	return mod, func() tea.Msg {
		ctx := context.Background()
		if err := s.Update(ctx, taskID, store.EditInput{Title: &title}); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

// --- tag operations ---

// splitTagInput splits input by spaces/commas — allows adding/removing
// several tags at once (e.g. "foo bar,baz").
func splitTagInput(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func (m Model) commitTagAdd(mod Model) (Model, tea.Cmd) {
	names := splitTagInput(mod.input)
	if len(names) == 0 || len(mod.tasks) == 0 {
		mod.mode = modeList
		mod.input = ""
		return mod, nil
	}
	taskID := mod.tasks[mod.cursor].ID
	mod.mode = modeList
	mod.input = ""
	s := mod.store
	t := mod.tab
	sq := mod.searchQuery
	tags := mergeContext(mod.context, mod.filterTags)
	return mod, func() tea.Msg {
		ctx := context.Background()
		if err := s.AddTaskTags(ctx, taskID, names); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

func (m Model) commitTagRm(mod Model) (Model, tea.Cmd) {
	names := splitTagInput(mod.input)
	if len(names) == 0 || len(mod.tasks) == 0 {
		mod.mode = modeList
		mod.input = ""
		return mod, nil
	}
	taskID := mod.tasks[mod.cursor].ID
	mod.mode = modeList
	mod.input = ""
	s := mod.store
	t := mod.tab
	sq := mod.searchQuery
	tags := mergeContext(mod.context, mod.filterTags)
	return mod, func() tea.Msg {
		ctx := context.Background()
		if err := s.RemoveTaskTags(ctx, taskID, names); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

func (m Model) commitTagFilter(mod Model) (Model, tea.Cmd) {
	tagName := strings.TrimSpace(mod.input)
	mod.mode = modeList
	mod.input = ""
	mod.cursor = 0
	if tagName == "" {
		// Empty input — clear tag filter.
		mod.filterTags = nil
	} else {
		mod.filterTags = []string{tagName}
	}
	return mod, mod.fetchForTab()
}

// --- confirm mode ---

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := m.confirmAction
		m.mode = modeList
		m.confirmMsg = ""
		m.confirmAction = nil
		if action != nil {
			return m, func() tea.Msg { return action() }
		}
	case "n", "N", "esc":
		m.mode = modeList
		m.confirmMsg = ""
		m.confirmAction = nil
	}
	return m, nil
}

// --- list mode ---

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tab == tabWorklog {
		return m.updateWorklog(msg)
	}
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.tasks)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.tasks)-1)
	case "tab", "right":
		m.tab = (m.tab + 1) % tabCount
		m.cursor = 0
		return m, m.fetchForTab()
	case "shift+tab", "left":
		m.tab = (m.tab + tabCount - 1) % tabCount
		m.cursor = 0
		return m, m.fetchForTab()
	case "esc":
		if m.searchQuery != "" || len(m.filterTags) > 0 {
			m.searchQuery = ""
			m.filterTags = nil
			m.cursor = 0
			return m, m.fetchForTab()
		}
	case "enter":
		if len(m.tasks) > 0 {
			m.mode = modeDetail
			return m, m.fetchDetailLog(m.tasks[m.cursor].ID)
		}
	case "t":
		return m.cycleStatus()
	case "x":
		return m.markDone()
	case "1":
		return m.setPriority(task.PriorityHigh)
	case "2":
		return m.setPriority(task.PriorityNormal)
	case "3":
		return m.setPriority(task.PriorityLow)
	case "/":
		m.startInput(modeSearch, m.searchQuery)
		return m, nil
	case "a":
		m.startInput(modeAdd, "")
		return m, nil
	case "e":
		if len(m.tasks) > 0 {
			m.startInput(modeEdit, m.tasks[m.cursor].Title)
		}
	case "d":
		if len(m.tasks) > 0 {
			m = m.confirmDeleteTask()
		}
	case "b":
		if len(m.tasks) > 0 {
			tk := m.tasks[m.cursor]
			return m, launchBodyEditor(tk.ID, tk.Body)
		}
	case "p":
		if len(m.tasks) > 0 {
			tk := m.tasks[m.cursor]
			return m, m.toggleDraft(tk.ID, !tk.Draft)
		}
	case "K":
		return m.reorderTask(-1)
	case "J":
		return m.reorderTask(+1)
	case "+":
		if len(m.tasks) > 0 {
			m.startInput(modeTagAdd, "")
		}
	case "-":
		if len(m.tasks) > 0 {
			m.startInput(modeTagRm, "")
		}
	case "T":
		m.startInput(modeTagFilter, "")
	case "C":
		return m.openContextPicker()
	case "s":
		return m, m.toggleTimer()
	}
	return m, nil
}

// --- worklog tab ---

// updateWorklog handles keys in the worklog tab.
// Skip task-specific actions (add/edit/delete/priority/tag) — worklog is read-only.
func (m Model) updateWorklog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "j", "down":
		if n := worklogNavCount(m.worklogEntries, time.Now().UTC()); m.cursor < n-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, worklogNavCount(m.worklogEntries, time.Now().UTC())-1)
	case "tab", "right":
		m.tab = (m.tab + 1) % tabCount
		m.cursor = 0
		return m, m.fetchForTab()
	case "shift+tab", "left":
		m.tab = (m.tab + tabCount - 1) % tabCount
		m.cursor = 0
		return m, m.fetchForTab()
	case "d":
		m.worklogRangeField = 0
		m.startInput(modeWorklogRange, time.Now().Local().Format("2006-01-02"))
		return m, nil
	case "esc":
		if m.worklogRange == wrRange {
			m.worklogRange = wrWeek
			m.worklogCustomFrom = nil
			m.worklogCustomTo = nil
			m.cursor = 0
			return m, m.fetchWorklog()
		}
		if m.searchQuery != "" || len(m.filterTags) > 0 {
			m.searchQuery = ""
			m.filterTags = nil
			m.cursor = 0
			return m, m.fetchWorklog()
		}
	case "/":
		m.startInput(modeSearch, m.searchQuery)
		return m, nil
	case "T":
		m.startInput(modeTagFilter, "")
	case "C":
		return m.openContextPicker()
	case "s":
		return m, m.toggleTimer()
	}
	return m, nil
}

// --- detail mode ---

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Tab switches focus between the left and right columns.
	if msg.String() == "tab" {
		m.detailFocus = 1 - m.detailFocus
		if m.detailFocus == 1 {
			m.detailLogCursor = 0
		}
		return m, nil
	}
	// When focus is on the right column (log), keys operate on entries.
	if m.detailFocus == 1 {
		return m.updateDetailLog(msg)
	}
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "backspace":
		m.mode = modeList
		m.detailFocus = 0
	case "t":
		return m.cycleStatus()
	case "x":
		return m.markDone()
	case "1":
		return m.setPriority(task.PriorityHigh)
	case "2":
		return m.setPriority(task.PriorityNormal)
	case "3":
		return m.setPriority(task.PriorityLow)
	case "p":
		if len(m.tasks) > 0 {
			tk := m.tasks[m.cursor]
			return m, m.toggleDraft(tk.ID, !tk.Draft)
		}
	case "b":
		if len(m.tasks) > 0 {
			tk := m.tasks[m.cursor]
			return m, launchBodyEditor(tk.ID, tk.Body)
		}
	case "e":
		if len(m.tasks) > 0 {
			m.startInput(modeEdit, m.tasks[m.cursor].Title)
		}
	case "+":
		if len(m.tasks) > 0 {
			m.startInput(modeTagAdd, "")
		}
	case "-":
		if len(m.tasks) > 0 {
			m.startInput(modeTagRm, "")
		}
	case "s":
		return m, m.toggleTimer()
	case "d":
		if len(m.tasks) > 0 {
			m = m.confirmDeleteTask()
		}
	}
	return m, nil
}

// --- detail log focus (right column) ---

// viewIdx maps a logical index (in m.detailLog) → on-screen position (newest on top).
// detailLogCursor is indexed in view order (0 = newest).
func (m Model) logEntryAt(viewIdx int) (task.TimeEntry, bool) {
	n := len(m.detailLog)
	if viewIdx < 0 || viewIdx >= n {
		return task.TimeEntry{}, false
	}
	return m.detailLog[n-1-viewIdx], true
}

func (m Model) updateDetailLog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "backspace":
		m.mode = modeList
		m.detailFocus = 0
	case "j", "down":
		if m.detailLogCursor < len(m.detailLog)-1 {
			m.detailLogCursor++
		}
	case "k", "up":
		if m.detailLogCursor > 0 {
			m.detailLogCursor--
		}
	case "g", "home":
		m.detailLogCursor = 0
	case "G", "end":
		m.detailLogCursor = max(0, len(m.detailLog)-1)
	case "d":
		entry, ok := m.logEntryAt(m.detailLogCursor)
		if !ok {
			return m, nil
		}
		m.mode = modeConfirm
		m.confirmMsg = fmt.Sprintf("delete entry #%d? (y/n)", entry.ID)
		entryID := entry.ID
		s := m.store
		taskID := m.tasks[m.cursor].ID
		m.confirmAction = func() tea.Msg {
			ctx := context.Background()
			if err := s.DeleteTimeEntry(ctx, entryID); err != nil {
				return errMsg{err}
			}
			entries, err := s.TimeEntries(ctx, taskID)
			if err != nil {
				return errMsg{err}
			}
			return detailLogMsg{taskID: taskID, entries: entries}
		}
	case "e":
		entry, ok := m.logEntryAt(m.detailLogCursor)
		if !ok {
			return m, nil
		}
		return m.startLogEdit(entry), nil
	case "a":
		return m.startLogAdd(), nil
	}
	return m, nil
}

// startLogAdd initializes the add-entry wizard (entryID=0 = sentinel "new").
// Pre-fill: today's date; start/end/note empty — user must fill them in.
func (m Model) startLogAdd() Model {
	m.logEditEntryID = 0
	m.logEditField = 0
	m.logEditStart = time.Now()
	m.logEditEnd = time.Time{}
	m.logEditNote = ""
	m.startInput(modeLogEdit, time.Now().Local().Format("2006-01-02"))
	return m
}

func (m Model) startLogEdit(entry task.TimeEntry) Model {
	m.logEditEntryID = entry.ID
	m.logEditField = 0
	m.logEditStart = entry.StartedAt
	m.logEditNote = entry.Note
	if entry.EndedAt != nil {
		m.logEditEnd = *entry.EndedAt
	} else {
		m.logEditEnd = time.Time{}
	}
	m.startInput(modeLogEdit, entry.StartedAt.Local().Format("2006-01-02"))
	return m
}

// commitLogEditField parses the current field, saves it to the buffer, and advances to the next.
// Sekwencja: 0=date → 1=start → 2=end → 3=note.
func (m Model) commitLogEditField(mod Model) (Model, tea.Cmd) {
	input := strings.TrimSpace(mod.input)

	switch mod.logEditField {
	case 0: // date — YYYY-MM-DD, reference for subsequent fields
		if input != "" {
			targetDay, err := time.ParseInLocation("2006-01-02", input, time.Local)
			if err != nil {
				mod.err = fmt.Errorf("expected YYYY-MM-DD: %w", err)
				return mod, nil
			}
			y, mo, d := targetDay.Date()
			orig := mod.logEditStart.Local()
			mod.logEditStart = time.Date(y, mo, d, orig.Hour(), orig.Minute(), orig.Second(), 0, time.Local)
			if !mod.logEditEnd.IsZero() {
				origEnd := mod.logEditEnd.Local()
				mod.logEditEnd = time.Date(y, mo, d, origEnd.Hour(), origEnd.Minute(), origEnd.Second(), 0, time.Local)
			}
		}
		mod.logEditField = 1
		mod.startInput(modeLogEdit, mod.logEditStart.Local().Format("15:04"))
		return mod, nil

	case 1: // start — reference is the (updated) entry date, not today
		if input != "" {
			t, err := timefmt.ParseFlag(input, mod.logEditStart)
			if err != nil {
				mod.err = err
				return mod, nil
			}
			mod.logEditStart = t
		}
		mod.logEditField = 2
		prefill := ""
		if !mod.logEditEnd.IsZero() {
			prefill = mod.logEditEnd.Local().Format("15:04")
		}
		mod.startInput(modeLogEdit, prefill)
		return mod, nil

	case 2: // end — reference is the (updated) start, not today
		if input != "" {
			t, err := timefmt.ParseFlag(input, mod.logEditStart)
			if err != nil {
				mod.err = err
				return mod, nil
			}
			mod.logEditEnd = t
		}
		mod.logEditField = 3
		mod.startInput(modeLogEdit, mod.logEditNote)
		return mod, nil

	case 3: // note — ostatnie pole; commit.
		entryID := mod.logEditEntryID
		taskID := mod.tasks[mod.cursor].ID
		s := mod.store
		start := mod.logEditStart
		note := input
		if entryID == 0 {
			// Add: End is required in CreateTimeEntryInput.
			if mod.logEditEnd.IsZero() {
				mod.err = fmt.Errorf("end time is required for a new entry")
				mod.logEditField = 2
				mod.startInput(modeLogEdit, "")
				return mod, nil
			}
			end := mod.logEditEnd
			mod.mode = modeDetail
			mod.input = ""
			mod.logEditField = 0
			return mod, func() tea.Msg {
				ctx := context.Background()
				if _, err := s.CreateTimeEntry(ctx, store.CreateTimeEntryInput{
					TaskID: taskID, Start: start, End: end, Note: note,
				}); err != nil {
					return errMsg{err}
				}
				entries, err := s.TimeEntries(ctx, taskID)
				if err != nil {
					return errMsg{err}
				}
				return detailLogMsg{taskID: taskID, entries: entries}
			}
		}
		var endPtr *time.Time
		if !mod.logEditEnd.IsZero() {
			e := mod.logEditEnd
			endPtr = &e
		}
		var notePtr *string
		if note != "" {
			notePtr = &note
		}
		mod.mode = modeDetail
		mod.input = ""
		mod.logEditField = 0
		return mod, func() tea.Msg {
			ctx := context.Background()
			in := store.UpdateTimeEntryInput{Start: &start, End: endPtr, Note: notePtr}
			if err := s.UpdateTimeEntry(ctx, entryID, in); err != nil {
				return errMsg{err}
			}
			entries, err := s.TimeEntries(ctx, taskID)
			if err != nil {
				return errMsg{err}
			}
			return detailLogMsg{taskID: taskID, entries: entries}
		}
	}
	return mod, nil
}

func (m Model) cancelLogEdit(mod Model) (Model, tea.Cmd) {
	mod.mode = modeDetail
	mod.input = ""
	mod.logEditField = 0
	return mod, nil
}

func (m Model) commitWorklogRangeField(mod Model) (Model, tea.Cmd) {
	input := strings.TrimSpace(mod.input)
	now := time.Now()

	switch mod.worklogRangeField {
	case 0: // from
		if input == "" {
			d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			offset := int(d.Weekday()) - 1
			if offset < 0 {
				offset = 6
			}
			start := d.AddDate(0, 0, -offset)
			mod.worklogCustomFrom = &start
		} else {
			t, err := timefmt.ParseFlag(input, now)
			if err != nil {
				mod.err = err
				return mod, nil
			}
			mod.worklogCustomFrom = &t
		}
		mod.worklogRangeField = 1
		mod.startInput(modeWorklogRange, now.Local().Format("2006-01-02"))
		return mod, nil

	case 1: // to — po zatwierdzeniu fetchujemy
		if input == "" {
			y, mo, d := now.Date()
			end := time.Date(y, mo, d+1, 0, 0, 0, 0, now.Location())
			mod.worklogCustomTo = &end
		} else {
			t, err := timefmt.ParseFlag(input, now)
			if err != nil {
				mod.err = err
				return mod, nil
			}
			// If a date-only was given (midnight), push to end of day (next midnight).
			if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
				t = t.Add(24 * time.Hour)
			}
			mod.worklogCustomTo = &t
		}
		mod.worklogRange = wrRange
		mod.mode = modeList
		mod.input = ""
		mod.worklogRangeField = 0
		mod.cursor = 0
		return mod, mod.fetchWorklog()
	}
	return mod, nil
}

func (m Model) cancelWorklogRange(mod Model) (Model, tea.Cmd) {
	mod.mode = modeList
	mod.input = ""
	mod.worklogRangeField = 0
	return mod, nil
}

// --- shared actions ---

// toggleTimer: if a timer is active → stop; otherwise → start on the cursor.
// Returns current timer state as activeTimerMsg so the header refreshes.
func (m Model) toggleTimer() tea.Cmd {
	s := m.store
	if m.active != nil {
		return func() tea.Msg {
			if _, err := s.StopTimer(context.Background(), nil); err != nil {
				return errMsg{err}
			}
			return activeTimerMsg{}
		}
	}
	if len(m.tasks) == 0 {
		return nil
	}
	tk := m.tasks[m.cursor]
	return func() tea.Msg {
		entry, err := s.StartTimer(context.Background(), tk.ID, "")
		if err != nil {
			return errMsg{err}
		}
		return activeTimerMsg{entry: &entry, title: tk.Title}
	}
}

func (m Model) setPriority(prio task.Priority) (Model, tea.Cmd) {
	if len(m.tasks) == 0 {
		return m, nil
	}
	tk := m.tasks[m.cursor]
	if tk.Priority == prio {
		return m, nil
	}
	s := m.store
	t := m.tab
	sq := m.searchQuery
	tags := mergeContext(m.context, m.filterTags)
	id := tk.ID
	return m, func() tea.Msg {
		ctx := context.Background()
		if err := s.Update(ctx, id, store.EditInput{Priority: &prio}); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

func (m Model) cycleStatus() (Model, tea.Cmd) {
	if len(m.tasks) == 0 {
		return m, nil
	}
	t := m.tasks[m.cursor]
	// t cycles todo → doing → action → todo; from done reopen to todo. Done lives under `x`.
	var next task.Status
	switch t.Status {
	case task.StatusTodo:
		next = task.StatusDoing
	case task.StatusDoing:
		next = task.StatusAction
	default:
		next = task.StatusTodo
	}
	return m, m.moveTask(t.ID, next)
}

func (m Model) markDone() (Model, tea.Cmd) {
	if len(m.tasks) == 0 {
		return m, nil
	}
	t := m.tasks[m.cursor]
	if t.Status == task.StatusDone {
		return m, nil
	}
	return m, m.moveTask(t.ID, task.StatusDone)
}

func (m Model) confirmDeleteTask() Model {
	tk := m.tasks[m.cursor]
	m.mode = modeConfirm
	m.confirmMsg = fmt.Sprintf("delete #%d %q? (y/n)", tk.ID, tk.Title)
	taskID := tk.ID
	s := m.store
	t := m.tab
	sq := m.searchQuery
	tags := mergeContext(m.context, m.filterTags)
	m.confirmAction = func() tea.Msg {
		ctx := context.Background()
		if err := s.Delete(ctx, taskID); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
	return m
}

func (m Model) moveTask(id int64, status task.Status) tea.Cmd {
	s := m.store
	t := m.tab
	sq := m.searchQuery
	tags := mergeContext(m.context, m.filterTags)
	return func() tea.Msg {
		ctx := context.Background()
		if err := s.Move(ctx, id, status); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

func (m Model) reorderTask(delta int) (Model, tea.Cmd) {
	target := m.cursor + delta
	if len(m.tasks) < 2 || target < 0 || target >= len(m.tasks) {
		return m, nil
	}

	// Swap in the local list.
	m.tasks[m.cursor], m.tasks[target] = m.tasks[target], m.tasks[m.cursor]
	m.cursor = target

	// Collect IDs in new order and persist positions.
	ids := make([]int64, len(m.tasks))
	for i, t := range m.tasks {
		ids[i] = t.ID
	}
	s := m.store
	return m, func() tea.Msg {
		ctx := context.Background()
		if err := s.SetPositions(ctx, ids); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) toggleDraft(id int64, draft bool) tea.Cmd {
	s := m.store
	t := m.tab
	sq := m.searchQuery
	tags := mergeContext(m.context, m.filterTags)
	return func() tea.Msg {
		ctx := context.Background()
		if err := s.Update(ctx, id, store.EditInput{Draft: &draft}); err != nil {
			return errMsg{err}
		}
		return fetchTaskList(ctx, s, sq, t, tags)
	}
}

// --- context picker ---

func (m Model) openContextPicker() (Model, tea.Cmd) {
	s := m.store
	return m, func() tea.Msg {
		tags, err := s.Tags(context.Background())
		if err != nil {
			return errMsg{err}
		}
		return contextTagsMsg(tags)
	}
}

type contextTagsMsg []string

func (m Model) filteredContextTags() []string {
	if m.contextSearch == "" {
		return m.contextTags
	}
	q := strings.ToLower(m.contextSearch)
	var out []string
	for _, t := range m.contextTags {
		if strings.Contains(strings.ToLower(t), q) {
			out = append(out, t)
		}
	}
	return out
}

func (m Model) updateContextPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filteredContextTags()
	// filtered + 1 entry for "— none (all)"
	total := len(filtered) + 1
	switch msg.Type {
	case tea.KeyRunes:
		m.contextSearch += string(msg.Runes)
		m.contextCursor = 0
	case tea.KeyBackspace:
		if m.contextSearch != "" {
			m.contextSearch = m.contextSearch[:len(m.contextSearch)-1]
			m.contextCursor = 0
		}
	case tea.KeyDown:
		if m.contextCursor < total-1 {
			m.contextCursor++
		}
	case tea.KeyUp:
		if m.contextCursor > 0 {
			m.contextCursor--
		}
	case tea.KeyEnter:
		filtered = m.filteredContextTags()
		if m.contextCursor < len(filtered) {
			m.context = filtered[m.contextCursor]
		} else {
			m.context = ""
		}
		m.contextSearch = ""
		m.mode = modeList
		m.cursor = 0
		if m.cfg != nil {
			m.cfg.Context = m.context
			m.cfg.Save()
		}
		return m, m.fetchForTab()
	case tea.KeyEscape:
		if m.contextSearch != "" {
			m.contextSearch = ""
			m.contextCursor = 0
		} else {
			m.mode = modeList
		}
	default:
		switch msg.String() {
		case "j":
			if m.contextCursor < total-1 {
				m.contextCursor++
			}
		case "k":
			if m.contextCursor > 0 {
				m.contextCursor--
			}
		case "q":
			m.contextSearch = ""
			m.mode = modeList
		}
	}
	return m, nil
}

func (m Model) renderContextPicker() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Context") + "\n")

	// Search line
	searchDisplay := m.contextSearch
	if searchDisplay == "" {
		searchDisplay = helpStyle.Render("type to filter…")
	} else {
		searchDisplay = inputStyle.Render(searchDisplay)
	}
	b.WriteString("  / " + searchDisplay + "\n\n")

	filtered := m.filteredContextTags()
	total := len(filtered) + 1
	for i := 0; i < total; i++ {
		prefix := "  "
		if i == m.contextCursor {
			prefix = selStyle.Render("▸ ")
		}
		var label string
		if i < len(filtered) {
			label = filtered[i]
			if filtered[i] == m.context {
				label = okStyle.Render(label + " ✓")
			}
		} else {
			label = helpStyle.Render("— none (all)")
			if m.context == "" {
				label = okStyle.Render("— none (all) ✓")
			}
		}
		b.WriteString(prefix + label + "\n")
	}
	return b.String()
}

// --- view ---

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header with tabs.
	b.WriteString(m.renderTabs())
	if m.context != "" {
		b.WriteString("  ")
		b.WriteString(okStyle.Render("[" + m.context + "]"))
	}
	if m.searchQuery != "" && m.mode != modeSearch {
		b.WriteString("  ")
		b.WriteString(inputStyle.Render("/" + m.searchQuery))
	}
	if len(m.filterTags) > 0 && m.mode != modeTagFilter {
		b.WriteString("  ")
		for _, tag := range m.filterTags {
			b.WriteString(inputStyle.Render("+" + tag + " "))
		}
	}
	if m.active != nil {
		elapsed := timefmt.Clock(m.active.Duration(m.now))
		title := m.activeTitle
		if len(title) > 40 {
			title = title[:37] + "…"
		}
		b.WriteString("  ")
		b.WriteString(okStyle.Render(fmt.Sprintf("⏱ #%d %s %s", m.active.TaskID, elapsed, title)))
	}
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(warnStyle.Render("  error: "+m.err.Error()) + "\n")
	}
	if m.flash != "" {
		b.WriteString(okStyle.Render("  "+m.flash) + "\n")
	}

	switch m.mode {
	case modeConfirm:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(warnStyle.Render("  "+m.confirmMsg) + "\n")
	case modeDetail:
		b.WriteString(m.renderDetail())
	case modeSearch:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(m.renderInputLine("/"))
	case modeAdd:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(m.renderInputLine("add: "))
	case modeAddTags:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  title: "+m.pendingAddTitle) + "\n")
		b.WriteString(m.renderInputLine("tags: "))
	case modeEdit:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(m.renderInputLine("edit: "))
	case modeTagAdd:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(m.renderInputLine("tag+: "))
	case modeTagRm:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		if len(m.tasks) > 0 && len(m.tasks[m.cursor].Tags) > 0 {
			b.WriteString(helpStyle.Render("  tags: "+strings.Join(m.tasks[m.cursor].Tags, ", ")) + "\n")
		}
		b.WriteString(m.renderInputLine("tag-: "))
	case modeTagFilter:
		b.WriteString(m.renderMainList())
		b.WriteString("\n")
		b.WriteString(m.renderInputLine("filter tag: "))
	case modeContextPicker:
		b.WriteString(m.renderContextPicker())
	case modeLogEdit:
		b.WriteString(m.renderDetail())
		b.WriteString("\n")
		prompts := []string{"date:  ", "start: ", "end:   ", "note:  "}
		b.WriteString(m.renderInputLine(prompts[m.logEditField]))
	default:
		b.WriteString(m.renderMainList())
	}

	b.WriteString("\n")
	b.WriteString(m.renderHelp())
	return b.String()
}

func (m Model) renderInputLine(prefix string) string {
	// Render the cursor as an underlined character.
	before := m.input[:m.inputCursor]
	after := ""
	cursorChar := " "
	if m.inputCursor < len(m.input) {
		cursorChar = string(m.input[m.inputCursor])
		after = m.input[m.inputCursor+1:]
	}
	cursor := lipgloss.NewStyle().Underline(true).Render(cursorChar)
	return "  " + promptStyle.Render(prefix) + inputStyle.Render(before) + cursor + inputStyle.Render(after)
}

func (m Model) renderTabs() string {
	tabs := []tab{tabActive, tabDone, tabWorklog}
	parts := make([]string, len(tabs))
	for i, t := range tabs {
		label := fmt.Sprintf(" %s ", t.String())
		if t == m.tab {
			parts[i] = titleStyle.Render(label)
		} else {
			parts[i] = helpStyle.Render(label)
		}
	}
	return strings.Join(parts, "│")
}

func (m Model) renderList() string {
	if len(m.tasks) == 0 {
		return helpStyle.Render("  no tasks") + "\n"
	}

	now := time.Now().UTC()
	var b strings.Builder
	// How many tasks fit in the window (leave room for tabs + help + input + padding).
	visible := max(m.height-8, 3)

	// Scroll: keep cursor within the visible range.
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := min(start+visible, len(m.tasks))

	for i := start; i < end; i++ {
		t := m.tasks[i]
		cursor := "  "
		if i == m.cursor {
			cursor = selStyle.Render("▸ ")
		}

		id := fmt.Sprintf("#%-4d", t.ID)
		status := renderStatus(t.Status)
		prio := renderPrio(t.Priority)
		title := t.Title
		if t.Draft {
			title = draftStyle.Render("~ " + title)
		}

		var extra []string
		if len(t.Tags) > 0 {
			for _, tag := range t.Tags {
				extra = append(extra, helpStyle.Render("+"+tag))
			}
		}
		isOverdue := t.DueAt != nil && t.DueAt.Before(now) && t.Status != task.StatusDone
		if t.DueAt != nil {
			ds := t.DueAt.Local().Format("01-02")
			if isOverdue {
				ds = dueWarn.Render("!" + ds)
			} else {
				ds = helpStyle.Render("@" + ds)
			}
			extra = append(extra, ds)
		}
		if m.active != nil && m.active.TaskID == t.ID {
			extra = append(extra, timerActive.Render("⏱"))
		} else if m.hasEntries[t.ID] {
			extra = append(extra, timerHas.Render("⏱"))
		}

		line := fmt.Sprintf("%s%s %s %s %s", cursor, helpStyle.Render(id), status, prio, title)
		if len(extra) > 0 {
			line += " " + strings.Join(extra, " ")
		}
		b.WriteString(line + "\n")
	}

	if len(m.tasks) > visible {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  … %d/%d", m.cursor+1, len(m.tasks))))
		b.WriteString("\n")
	}
	return b.String()
}

// renderMainList picks a renderer based on the active tab.
func (m Model) renderMainList() string {
	if m.tab == tabWorklog {
		return m.renderWorklogList()
	}
	return m.renderList()
}

// wlRow is one row in the worklog view — either a day header or a task row.
type wlRow struct {
	header   bool
	label    string // day header
	taskID   int64
	title    string
	duration time.Duration
	count    int
	navIdx   int // index in the sequence of navigable rows (non-headers only)
}

// buildWorklogRows aggregates entries into a flat list of rows (day headers + tasks).
func buildWorklogRows(entries []store.WorklogEntry, now time.Time) (rows []wlRow, navCount int) {
	type taskAgg struct {
		id       int64
		title    string
		duration time.Duration
		count    int
	}
	type dayData struct {
		date  time.Time
		tasks map[int64]*taskAgg
		order []int64
		total time.Duration
	}
	days := make(map[string]*dayData)
	var dayOrder []string
	for _, e := range entries {
		local := e.StartedAt.Local()
		key := local.Format("2006-01-02")
		if _, ok := days[key]; !ok {
			y, mo, d := local.Date()
			days[key] = &dayData{
				date:  time.Date(y, mo, d, 0, 0, 0, 0, local.Location()),
				tasks: make(map[int64]*taskAgg),
			}
			dayOrder = append(dayOrder, key)
		}
		day := days[key]
		dur := e.Duration(now)
		day.total += dur
		if _, ok := day.tasks[e.TaskID]; !ok {
			day.tasks[e.TaskID] = &taskAgg{id: e.TaskID, title: e.TaskTitle}
			day.order = append(day.order, e.TaskID)
		}
		day.tasks[e.TaskID].duration += dur
		day.tasks[e.TaskID].count++
	}
	sort.Strings(dayOrder)
	for _, key := range dayOrder {
		day := days[key]
		label := fmt.Sprintf("%s %d %s  — %s",
			day.date.Weekday(), day.date.Day(), day.date.Format("Jan"),
			timefmt.Clock(day.total))
		rows = append(rows, wlRow{header: true, label: label})
		for _, id := range day.order {
			t := day.tasks[id]
			rows = append(rows, wlRow{
				taskID: t.id, title: t.title, duration: t.duration, count: t.count,
				navIdx: navCount,
			})
			navCount++
		}
	}
	return
}

func worklogNavCount(entries []store.WorklogEntry, now time.Time) int {
	_, n := buildWorklogRows(entries, now)
	return n
}

var dayHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))

func (m Model) worklogRangeLabel() string {
	switch m.worklogRange {
	case wrRange:
		from := "?"
		to := "?"
		if m.worklogCustomFrom != nil {
			from = m.worklogCustomFrom.Local().Format("2006-01-02")
		}
		if m.worklogCustomTo != nil {
			to = m.worklogCustomTo.Local().Format("2006-01-02")
		}
		return fmt.Sprintf("%s – %s", from, to)
	default: // wrWeek
		now := time.Now()
		d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		offset := int(d.Weekday()) - 1
		if offset < 0 {
			offset = 6
		}
		mon := d.AddDate(0, 0, -offset)
		sun := mon.AddDate(0, 0, 6)
		return fmt.Sprintf("week %s – %s", mon.Format("02 Jan"), sun.Format("02 Jan"))
	}
}

func (m Model) renderWorklogList() string {
	now := time.Now().UTC()
	var b strings.Builder

	if m.mode == modeWorklogRange {
		prompt := "from (YYYY-MM-DD):"
		if m.worklogRangeField == 1 {
			prompt = "to (YYYY-MM-DD):"
		}
		b.WriteString(promptStyle.Render("  "+prompt) + "  " + inputStyle.Render(m.input+"_"))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(helpStyle.Render("  " + m.worklogRangeLabel()))
	b.WriteString("\n\n")

	if len(m.worklogEntries) == 0 {
		b.WriteString(helpStyle.Render("  no time entries"))
		b.WriteString("\n")
		return b.String()
	}

	rows, navCount := buildWorklogRows(m.worklogEntries, now)

	// Find the display-index of the selected row.
	selectedDisplay := 0
	for i, r := range rows {
		if !r.header && r.navIdx == m.cursor {
			selectedDisplay = i
			break
		}
	}

	var total time.Duration
	for _, e := range m.worklogEntries {
		total += e.Duration(now)
	}

	visible := max(m.height-10, 3)
	start := 0
	if selectedDisplay >= visible {
		start = selectedDisplay - visible + 1
	}
	end := min(start+visible, len(rows))

	for i := start; i < end; i++ {
		r := rows[i]
		if r.header {
			b.WriteString("  " + dayHeaderStyle.Render(r.label) + "\n")
			continue
		}
		cur := "  "
		if r.navIdx == m.cursor {
			cur = selStyle.Render("▸ ")
		}
		title := r.title
		if len(title) > 32 {
			title = title[:29] + "…"
		}
		s := "sessions"
		if r.count == 1 {
			s = "session"
		}
		line := fmt.Sprintf("%s#%-4d %-32s  %s  %d %s",
			cur, r.taskID, title, timefmt.Clock(r.duration), r.count, s)
		b.WriteString(line + "\n")
	}

	if navCount > 1 {
		b.WriteString(helpStyle.Render(fmt.Sprintf("  … %d/%d", m.cursor+1, navCount)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("  total: %s (%d sessions)",
		timefmt.Clock(total), len(m.worklogEntries))))
	b.WriteString("\n")
	return b.String()
}

func (m Model) renderDetail() string {
	t := m.tasks[m.cursor]
	now := time.Now().UTC()

	rightW := min(max(m.width/2, 42), 70)
	leftW := max(m.width-rightW-4, 20)

	// --- left column: metadata + body ---
	var left strings.Builder
	left.WriteString(titleStyle.Render(fmt.Sprintf("#%d %s", t.ID, t.Title)))
	left.WriteString("\n\n")

	row := func(key, val string) {
		left.WriteString(detailKey.Render(key))
		left.WriteString(val)
		left.WriteString("\n")
	}
	row("Status", renderStatus(t.Status))
	row("Priority", renderPrio(t.Priority))
	if t.Draft {
		row("Draft", "yes")
	}
	if len(t.Tags) > 0 {
		row("Tags", strings.Join(t.Tags, ", "))
	}
	if t.DueAt != nil {
		ds := t.DueAt.Local().Format("2006-01-02")
		if t.DueAt.Before(now) && t.Status != task.StatusDone {
			ds = dueWarn.Render(ds + " (overdue)")
		}
		row("Due", ds)
	}
	row("Created", t.CreatedAt.Local().Format("2006-01-02 15:04"))

	if t.Body != "" {
		left.WriteString("\n")
		if m.mdRenderer != nil {
			if rendered, err := m.mdRenderer.Render(t.Body); err == nil {
				left.WriteString(strings.TrimRight(rendered, "\n"))
			} else {
				left.WriteString(t.Body)
			}
		} else {
			left.WriteString(t.Body)
		}
		left.WriteString("\n")
	}

	// --- right column: time log ---
	right := m.renderDetailLog(t.ID, now)

	// Layout ~45/55 — right column (log) wider so the note fits after the entry.

	leftCol := lipgloss.NewStyle().Width(leftW).Padding(0, 1).Render(left.String())
	borderColor := lipgloss.Color("8")
	if m.detailFocus == 1 {
		borderColor = lipgloss.Color("14")
	}
	rightCol := lipgloss.NewStyle().Width(rightW).Padding(0, 1).
		BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Render(right)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
}

func (m Model) renderDetailLog(taskID int64, now time.Time) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Time log"))
	b.WriteString("\n")

	// If the log hasn't loaded yet or is for a different task.
	if m.detailLogTaskID != taskID {
		b.WriteString(helpStyle.Render("loading…"))
		return b.String()
	}
	if len(m.detailLog) == 0 {
		b.WriteString(helpStyle.Render("no entries"))
		return b.String()
	}

	var total time.Duration
	for _, e := range m.detailLog {
		total += e.Duration(now)
	}
	b.WriteString(helpStyle.Render(fmt.Sprintf("total: %s\n", timefmt.Clock(total))))
	b.WriteString("\n")

	// Newest on top.
	viewIdx := 0
	for i := len(m.detailLog) - 1; i >= 0; i-- {
		e := m.detailLog[i]
		date := e.StartedAt.Local().Format("01-02")
		startT := e.StartedAt.Local().Format("15:04")
		endT := "..."
		if e.EndedAt != nil {
			endT = e.EndedAt.Local().Format("15:04")
		}
		prefix := "  "
		if m.detailFocus == 1 && m.mode == modeDetail && viewIdx == m.detailLogCursor {
			prefix = selStyle.Render("▸ ")
		}
		line := fmt.Sprintf("%s#%-3d %s %s-%s %s",
			prefix, e.ID, date, startT, endT, timefmt.Clock(e.Duration(now)))
		switch {
		case e.Active():
			b.WriteString(okStyle.Render(line))
		case m.detailFocus == 1 && viewIdx == m.detailLogCursor:
			b.WriteString(inputStyle.Render(line))
		default:
			b.WriteString(line)
		}
		if e.Note != "" {
			b.WriteString("  " + helpStyle.Render(e.Note))
		}
		b.WriteString("\n")
		viewIdx++
	}
	return b.String()
}

func (m Model) renderHelp() string {
	switch m.mode {
	case modeSearch:
		return m.helpParts("type to search", "enter confirm", "esc clear")
	case modeAdd:
		return m.helpParts("type title", "enter next (tags)", "esc cancel")
	case modeAddTags:
		return m.helpParts("type tag name(s) — space/comma separated", "enter add", "esc cancel (empty = no tags)")
	case modeEdit:
		return m.helpParts("edit title", "enter save", "esc cancel")
	case modeTagAdd:
		return m.helpParts("type tag name(s) — space/comma separated", "enter add", "esc cancel")
	case modeTagRm:
		return m.helpParts("type tag name(s) — space/comma separated", "enter remove", "esc cancel")
	case modeTagFilter:
		return m.helpParts("type tag name", "enter filter", "esc cancel (empty = clear)")
	case modeContextPicker:
		return m.helpParts("type to filter", "j/k nav", "enter select", "esc clear/cancel")
	case modeConfirm:
		return m.helpParts("y confirm", "n/esc cancel")
	case modeDetail:
		if m.detailFocus == 1 {
			return m.helpParts("tab switch", "j/k nav", "a add entry", "e edit entry", "d delete entry", "esc back")
		}
		return m.helpParts("tab log", "esc back", "t todo/doing", "x done", "1/2/3 prio", "e edit", "b body", "+/- tag", "s timer", "d delete", "p toggle draft", "q quit")
	case modeLogEdit:
		return m.helpParts("HH:MM / YYYY-MM-DD HH:MM / -1h30m", "enter next", "esc cancel (empty = keep)")
	default:
		if m.tab == tabWorklog {
			parts := []string{
				"↑/k ↓/j nav",
				"←/→ tab",
				"r range",
				"/ search",
				"T filter tag",
				"C context",
				"s timer",
			}
			if m.searchQuery != "" || len(m.filterTags) > 0 {
				parts = append(parts, "esc clear")
			}
			parts = append(parts, "q quit")
			return m.helpParts(parts...)
		}
		parts := []string{
			"↑/k ↓/j nav",
			"K/J reorder",
			"←/→ tab",
			"enter detail",
			"t todo/doing",
			"x done",
			"1/2/3 prio",
			"a add",
			"e edit",
			"b body",
			"+/- tag",
			"T filter tag",
			"C context",
			"s timer",
			"d del",
			"p pub",
			"/ search",
		}
		if m.searchQuery != "" || len(m.filterTags) > 0 {
			parts = append(parts, "esc clear")
		}
		parts = append(parts, "q quit")
		return m.helpParts(parts...)
	}
}

// helpParts renders a list of shortcuts joined by " • ", wrapping at m.width
// on separator boundaries. New lines get the same 2-space indent as the first.
func (m Model) helpParts(parts ...string) string {
	const indent = "  "
	const sep = " • "
	if len(parts) == 0 {
		return ""
	}
	width := m.width
	var b strings.Builder
	b.WriteString(indent)
	col := runeLen(indent)
	for i, p := range parts {
		pl := runeLen(p)
		if i == 0 {
			b.WriteString(p)
			col += pl
			continue
		}
		if width > 0 && col+runeLen(sep)+pl > width {
			b.WriteString("\n")
			b.WriteString(indent)
			b.WriteString(p)
			col = runeLen(indent) + pl
		} else {
			b.WriteString(sep)
			b.WriteString(p)
			col += runeLen(sep) + pl
		}
	}
	return helpStyle.Render(b.String())
}

func runeLen(s string) int { return len([]rune(s)) }

func renderStatus(s task.Status) string {
	switch s {
	case task.StatusDoing:
		return statusDo.Render(string(s))
	case task.StatusAction:
		return statusAction.Render(string(s))
	case task.StatusDone:
		return statusDone.Render(string(s))
	default:
		return statusTodo.Render(string(s))
	}
}

func renderPrio(p task.Priority) string {
	switch p {
	case task.PriorityHigh:
		return prioHigh.Render("⬆")
	case task.PriorityLow:
		return prioLow.Render("⬇")
	default:
		return " "
	}
}
