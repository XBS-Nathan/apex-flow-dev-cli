package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/db"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot [name]",
	Short: "Create a database snapshot",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		label := ""
		if len(args) > 0 {
			label = args[0]
		}

		fmt.Printf("Creating snapshot of %s...\n", p.Config.DB)
		store, err := db.NewStore(p.Config.DBConfig())
		if err != nil {
			return err
		}
		snapshotDir := db.SnapshotDir(p.Config.DB, label)
		if err := store.Snapshot(p.Config.DB, snapshotDir); err != nil {
			return err
		}

		fmt.Printf("✓ Snapshot saved: %s\n", snapshotDir)
		return nil
	},
}

var snapshotRestoreCmd = &cobra.Command{
	Use:   "restore [name]",
	Short: "Restore from a database snapshot",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		snapshots, err := db.ListSnapshots(p.Config.DB)
		if err != nil {
			return err
		}
		if len(snapshots) == 0 {
			return fmt.Errorf("no snapshots found for %s", p.Config.DB)
		}

		var snapshotPath string
		if len(args) > 0 {
			// Find snapshot by name
			for _, s := range snapshots {
				if filepath.Base(s) == args[0] {
					snapshotPath = s
					break
				}
			}
			if snapshotPath == "" {
				return fmt.Errorf("snapshot %q not found", args[0])
			}
		} else {
			// Use most recent
			sort.Strings(snapshots)
			snapshotPath = snapshots[len(snapshots)-1]
		}

		fmt.Printf("Restoring %s from %s...\n", p.Config.DB, filepath.Base(snapshotPath))
		store, err := db.NewStore(p.Config.DBConfig())
		if err != nil {
			return err
		}
		if err := store.Restore(p.Config.DB, snapshotPath); err != nil {
			return err
		}

		fmt.Printf("✓ Database %s restored\n", p.Config.DB)
		return nil
	},
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available snapshots",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		snapshots, err := db.ListSnapshots(p.Config.DB)
		if err != nil {
			return err
		}

		if len(snapshots) == 0 {
			fmt.Printf("No snapshots for %s\n", p.Config.DB)
			return nil
		}

		fmt.Printf("Snapshots for %s:\n", p.Config.DB)
		for _, s := range snapshots {
			fmt.Printf("  %s\n", filepath.Base(s))
		}
		return nil
	},
}
