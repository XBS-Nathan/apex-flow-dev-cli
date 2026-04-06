package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/nova/internal/phpimage"
	"github.com/XBS-Nathan/nova/internal/project"
)

func init() { rootCmd.AddCommand(buildCmd) }

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Rebuild PHP images",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		cfg := phpimage.ImageConfig{
			PHPVersion: p.Config.PHP,
			Extensions: p.Config.Extensions,
		}

		fmt.Printf("Building PHP %s...\n", p.Config.PHP)
		if err := phpimage.ForceBuild(cfg); err != nil {
			return err
		}

		fmt.Println("✓ PHP image built")
		return nil
	},
}
