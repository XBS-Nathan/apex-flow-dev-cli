package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(composerCmd)
}

var composerCmd = &cobra.Command{
	Use:                "composer [args...]",
	Short:              "Run composer with the project's PHP version",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		phpBin := p.PHPBin()

		composerPath, err := exec.LookPath("composer")
		if err != nil {
			return fmt.Errorf("composer not found in PATH")
		}
		composerArgs := append([]string{composerPath}, args...)

		c := exec.Command(phpBin, composerArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		c.Dir = p.Dir
		return c.Run()
	},
}
