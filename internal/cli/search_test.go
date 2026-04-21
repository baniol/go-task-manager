package cli

import (
	"strings"
	"testing"

	"go-task-manager/internal/store"
	"go-task-manager/internal/task"
)

func TestSearchRequiresQuery(t *testing.T) {
	h := newHarness(t)
	err := h.run("search")
	if err == nil {
		t.Fatal("want error for missing query")
	}
}

func TestSearchNoResults(t *testing.T) {
	h := newHarness(t)
	h.store.Add(h.ctx, store.AddInput{Title: "hello world", Ready: true})
	if err := h.run("search", "nonexistent"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "no tasks match") {
		t.Errorf("stdout = %q, want 'no tasks match'", h.stdout.String())
	}
}

func TestSearchFindsResults(t *testing.T) {
	h := newHarness(t)
	h.store.Add(h.ctx, store.AddInput{Title: "deploy backend", Ready: true})
	h.store.Add(h.ctx, store.AddInput{Title: "fix frontend", Ready: true})

	if err := h.run("search", "backend"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "deploy backend") {
		t.Errorf("want 'deploy backend' in output, got %q", out)
	}
	if strings.Contains(out, "fix frontend") {
		t.Errorf("unexpected 'fix frontend' in output")
	}
}

func TestSearchWithFlags(t *testing.T) {
	h := newHarness(t)
	h.store.Add(h.ctx, store.AddInput{
		Title: "deploy api", Tags: []string{"ops"}, Ready: true,
	})
	h.store.Add(h.ctx, store.AddInput{
		Title: "deploy web", Tags: []string{"web"}, Ready: true,
	})

	// --tag ops
	if err := h.run("search", "deploy", "--tag", "ops"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "deploy api") {
		t.Errorf("want 'deploy api' with --tag ops, got %q", out)
	}
	if strings.Contains(out, "deploy web") {
		t.Errorf("unexpected 'deploy web' with --tag ops")
	}

	// --status doing (nothing is doing)
	if err := h.run("search", "deploy", "--status", "doing"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(h.stdout.String(), "no tasks match") {
		t.Errorf("want no results for --status doing")
	}
}

func TestSearchInvalidStatus(t *testing.T) {
	h := newHarness(t)
	err := h.run("search", "foo", "--status", "invalid")
	if err == nil {
		t.Fatal("want error for invalid status")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Errorf("err = %q, want 'invalid status'", err.Error())
	}
}

func TestSearchMultiWordQuery(t *testing.T) {
	h := newHarness(t)
	h.store.Add(h.ctx, store.AddInput{Title: "fix critical bug in auth", Ready: true})
	h.store.Add(h.ctx, store.AddInput{Title: "add auth middleware", Ready: true})

	// Search "critical auth" — both words (implicit AND in FTS5)
	if err := h.run("search", "critical", "auth"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "fix critical bug in auth") {
		t.Errorf("want 'fix critical bug in auth', got %q", out)
	}
	// "add auth middleware" has no "critical"
	if strings.Contains(out, "add auth middleware") {
		t.Errorf("unexpected 'add auth middleware' in multi-word search")
	}
}

// Verifies that search sees the body, not just the title.
func TestSearchMatchesBody(t *testing.T) {
	h := newHarness(t)
	h.store.Add(h.ctx, store.AddInput{
		Title: "fix issue", Body: "playbook deploy steps", Ready: true,
	})

	if err := h.run("search", "playbook"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.stdout.String()
	if !strings.Contains(out, "fix issue") {
		t.Errorf("want search to match body, got %q", out)
	}
}

// Unused import guard — make sure task and store stay imported.
var _ = task.StatusTodo
