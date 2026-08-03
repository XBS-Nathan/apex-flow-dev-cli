// Package dns configures the shared wildcard DNS responder for .test
// domains. It lets other machines (e.g. over Tailscale) resolve Nova
// sites to this machine without per-host /etc/hosts entries.
package dns

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectListenIP returns the address the DNS responder should bind to.
// A non-empty override wins; otherwise the machine's Tailscale IPv4 is
// detected via `tailscale ip -4`.
func DetectListenIP(override string) (string, error) {
	if override != "" {
		if net.ParseIP(override) == nil {
			return "", fmt.Errorf("dns_bind %q is not a valid IP address", override)
		}
		return override, nil
	}

	path, err := exec.LookPath("tailscale")
	if err != nil {
		return "", fmt.Errorf("tailscale not found in PATH — set dns_bind in ~/.nova/config.yaml")
	}
	out, err := exec.Command(path, "ip", "-4").Output()
	if err != nil {
		return "", fmt.Errorf("detecting Tailscale IP (is Tailscale up?): %w", err)
	}
	return parseTailscaleIP(string(out))
}

// parseTailscaleIP extracts the first IPv4 address from `tailscale ip -4` output.
func parseTailscaleIP(out string) (string, error) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ip := net.ParseIP(line)
		if ip == nil || ip.To4() == nil {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no IPv4 address in tailscale output %q", strings.TrimSpace(out))
}

// WriteCorefile writes the CoreDNS config that answers every .test query
// with ip. The file lands in <globalDir>/dns/Corefile, which the compose
// file mounts into the dns container.
func WriteCorefile(globalDir, ip string) error {
	dir := filepath.Join(globalDir, "dns")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating dns config dir: %w", err)
	}
	path := filepath.Join(dir, "Corefile")
	if err := os.WriteFile(path, []byte(corefile(ip)), 0644); err != nil {
		return fmt.Errorf("writing Corefile: %w", err)
	}
	return nil
}

// corefile renders the CoreDNS config. AAAA queries get an empty NOERROR
// so clients fall through to the A record instead of treating the name
// as nonexistent. reload picks up IP changes (e.g. a new Tailscale IP)
// without a container restart.
func corefile(ip string) string {
	var b strings.Builder
	b.WriteString("test:53 {\n")
	b.WriteString("\terrors\n")
	b.WriteString("\treload\n")
	b.WriteString("\ttemplate IN A {\n")
	fmt.Fprintf(&b, "\t\tanswer \"{{ .Name }} 300 IN A %s\"\n", ip)
	b.WriteString("\t}\n")
	b.WriteString("\ttemplate IN AAAA {\n")
	b.WriteString("\t\trcode NOERROR\n")
	b.WriteString("\t}\n")
	// Apple clients query HTTPS/SVCB records before A; without a template
	// those fall off the plugin chain as SERVFAIL, which macOS/iOS treat
	// as resolution failure even though A lookups succeed.
	b.WriteString("\ttemplate IN HTTPS {\n")
	b.WriteString("\t\trcode NOERROR\n")
	b.WriteString("\t}\n")
	b.WriteString("\ttemplate IN SVCB {\n")
	b.WriteString("\t\trcode NOERROR\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}
