package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"go-task-manager/internal/store"
)

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: %w", s, err)
	}
	return id, nil
}

func (a *App) printTask(c *cobra.Command, id int64) error {
	t, err := a.store.Get(c.Context(), id)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.OutOrStdout(), "task #%d %s\n", t.ID, formatTaskShort(t))
	return nil
}

type App struct {
	store store.Store
}

// version ustawiana przez linker: -ldflags "-X go-task-manager/internal/cli.version=..."
var version = "dev"

func NewRootCmd(s store.Store) *cobra.Command {
	a := &App{store: s}
	cmd := &cobra.Command{
		Use:           "tm",
		Short:         "tm — task manager",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.SetVersionTemplate("tm {{.Version}}\n")
	cmd.AddCommand(
		a.newAddCmd(),
		a.newListCmd(),
		a.newSearchCmd(),
		a.newUICmd(),
		a.newMoveCmd(),
		a.newEditCmd(),
		a.newRmCmd(),
		a.newPublishCmd(),
		a.newTagCmd(),
		a.newStartCmd(),
		a.newStopCmd(),
		a.newStatusCmd(),
		a.newLogCmd(),
		a.newWorklogCmd(),
		a.newExportCmd(),
		a.newImportCmd(),
		a.newBackupCmd(),
		a.newResetCmd(),
	)
	return cmd
}

// Execute runs the root command. Returns the exit code.
func Execute() int {
	ctx := context.Background()
	dbPath, err := defaultDBPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	s, err := store.OpenSQLite(ctx, dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer s.Close()

	if err := NewRootCmd(s).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func defaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "tasks.db"), nil
}
