package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"go-task-manager/internal/store"
)

// testHarness builds a fresh CLI backed by a temporary SQLite DB and lets
// tests run commands while capturing stdout/stderr and the returned error.
type testHarness struct {
	t      *testing.T
	store  *store.SQLite
	ctx    context.Context
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newHarness(t *testing.T) *testHarness {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tm.db")
	s, err := store.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return &testHarness{t: t, store: s, ctx: ctx}
}

// run builds root cmd, sets args and executes. Returns the Execute error.
// stdout/stderr buffers are reset before each call, so run() can be called
// multiple times within a single test.
func (h *testHarness) run(args ...string) error {
	h.stdout.Reset()
	h.stderr.Reset()
	root := NewRootCmd(h.store)
	root.SetArgs(args)
	root.SetOut(&h.stdout)
	root.SetErr(&h.stderr)
	return root.ExecuteContext(h.ctx)
}
