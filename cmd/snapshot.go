package cmd

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/config"
	"github.com/XBS-Nathan/nova/internal/db"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.AddCommand(snapshotRestoreCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.Flags().Bool("local", false, "Store snapshot in project .nova/ directory")
	snapshotRestoreCmd.Flags().Bool("local", false, "Restore from project .nova/ directory")
	snapshotListCmd.Flags().Bool("local", false, "List snapshots from project .nova/ directory")
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
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		svcName := dbServiceForProject(p.Config, global)
		store, err := db.NewStore(p.Config.DBConfig(), svcName)
		if err != nil {
			return err
		}

		local, _ := cmd.Flags().GetBool("local")
		snapshotDir := snapshotDirForFlags(p, local, p.Config.DB, label)
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

		local, _ := cmd.Flags().GetBool("local")
		snapshots, err := listSnapshotsForFlags(p, local, p.Config.DB)
		if err != nil {
			return err
		}
		if len(snapshots) == 0 {
			return fmt.Errorf("no snapshots found for %s", p.Config.DB)
		}

		var snapshotPath string
		if len(args) > 0 {
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
			sort.Strings(snapshots)
			options := make([]string, len(snapshots))
			for i, s := range snapshots {
				options[i] = filepath.Base(s)
			}
			selected, err := pterm.DefaultInteractiveSelect.
				WithOptions(options).
				WithDefaultOption(options[len(options)-1]).
				Show("Select snapshot to restore")
			if err != nil {
				return fmt.Errorf("selecting snapshot: %w", err)
			}
			for _, s := range snapshots {
				if filepath.Base(s) == selected {
					snapshotPath = s
					break
				}
			}
		}

		fmt.Printf("Restoring %s from %s...\n", p.Config.DB, filepath.Base(snapshotPath))
		global, err := config.LoadGlobal()
		if err != nil {
			return err
		}
		svcName := dbServiceForProject(p.Config, global)
		store, err := db.NewStore(p.Config.DBConfig(), svcName)
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

		local, _ := cmd.Flags().GetBool("local")
		snapshots, err := listSnapshotsForFlags(p, local, p.Config.DB)
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

// snapshotDirForFlags returns the snapshot directory based on --local flag.
func snapshotDirForFlags(p *project.Project, local bool, dbName, label string) string {
	if local {
		return db.LocalSnapshotDir(p.Dir, dbName, label)
	}
	return db.SnapshotDir(dbName, label)
}

// listSnapshotsForFlags returns snapshots from the appropriate location.
func listSnapshotsForFlags(p *project.Project, local bool, dbName string) ([]string, error) {
	if local {
		return db.ListLocalSnapshots(p.Dir, dbName)
	}
	return db.ListSnapshots(dbName)
}
