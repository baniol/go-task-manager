package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func (a *App) newBackupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup",
		Short: "dump the whole SQLite database to a file next to the original (with a timestamp)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			src := a.store.Path()
			if src == "" {
				return fmt.Errorf("backup: store has no file path")
			}
			dir := filepath.Dir(src)
			base := filepath.Base(src)
			ext := filepath.Ext(base)
			stem := strings.TrimSuffix(base, ext)
			ts := time.Now().Format("20060102-150405")
			dst := filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, ts, ext))

			if err := a.store.Backup(c.Context(), dst); err != nil {
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), dst)
			return nil
		},
	}
}
