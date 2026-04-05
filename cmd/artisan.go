package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/config"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/php"
	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(phpCmd)
	rootCmd.AddCommand(artisanCmd)
}

var phpCmd = &cobra.Command{
	Use:                "php [args...]",
	Short:              "Run PHP with the project's configured version",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		return php.Run(p.Config.PHP, args...)
	},
}

var artisanCmd = &cobra.Command{
	Use:                "artisan [args...]",
	Short:              "Run php artisan (Laravel projects only)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		if p.Config.Type != config.TypeLaravel {
			return fmt.Errorf(
				"artisan is only available for Laravel projects (current type: %s)",
				p.Config.Type,
			)
		}

		artisanArgs := append([]string{"artisan"}, args...)
		return php.Run(p.Config.PHP, artisanArgs...)
	},
}
