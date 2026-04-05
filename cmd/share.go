package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/XBS-Nathan/apex-flow-dev-cli/internal/project"
)

func init() {
	rootCmd.AddCommand(shareCmd)
}

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share the current project via tunnel",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := project.Detect()
		if err != nil {
			return err
		}

		domain := p.SiteDomain()

		// Try cloudflared first, fall back to ngrok
		if _, err := exec.LookPath("cloudflared"); err == nil {
			fmt.Printf("Sharing %s via Cloudflare Tunnel...\n", domain)
			c := exec.Command("cloudflared", "tunnel", "--url", fmt.Sprintf("http://%s", domain))
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			return c.Run()
		}

		if _, err := exec.LookPath("ngrok"); err == nil {
			fmt.Printf("Sharing %s via ngrok...\n", domain)
			c := exec.Command("ngrok", "http", fmt.Sprintf("--host-header=%s", domain), "80")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			return c.Run()
		}

		return fmt.Errorf("neither cloudflared nor ngrok found in PATH — install one to use dev share")
	},
}
