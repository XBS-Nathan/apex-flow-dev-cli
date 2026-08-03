package dns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTailscaleIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		out     string
		want    string
		wantErr bool
	}{
		{name: "single address", out: "100.101.102.103\n", want: "100.101.102.103"},
		{name: "leading blank line", out: "\n100.64.0.5\n", want: "100.64.0.5"},
		{name: "skips ipv6", out: "fd7a:115c:a1e0::1\n100.64.0.5\n", want: "100.64.0.5"},
		{name: "whitespace padding", out: "  100.64.0.5  \n", want: "100.64.0.5"},
		{name: "empty output", out: "", wantErr: true},
		{name: "garbage", out: "not-an-ip\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTailscaleIP(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTailscaleIP(%q) = %q, want error", tt.out, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTailscaleIP(%q) returned error: %v", tt.out, err)
			}
			if got != tt.want {
				t.Errorf("parseTailscaleIP(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

func TestDetectListenIP_Override(t *testing.T) {
	t.Parallel()

	got, err := DetectListenIP("192.168.1.50")
	if err != nil {
		t.Fatalf("DetectListenIP with valid override returned error: %v", err)
	}
	if got != "192.168.1.50" {
		t.Errorf("DetectListenIP = %q, want %q", got, "192.168.1.50")
	}
}

func TestDetectListenIP_InvalidOverride(t *testing.T) {
	t.Parallel()

	if _, err := DetectListenIP("not-an-ip"); err == nil {
		t.Fatal("DetectListenIP with invalid override should return error")
	}
}

func TestCorefile(t *testing.T) {
	t.Parallel()

	got := corefile("100.64.0.5")

	for _, want := range []string{
		"test:53 {",
		`answer "{{ .Name }} 300 IN A 100.64.0.5"`,
		"template IN AAAA",
		"template IN HTTPS",
		"template IN SVCB",
		"rcode NOERROR",
		"reload",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("corefile missing %q; got:\n%s", want, got)
		}
	}
}

func TestWriteCorefile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := WriteCorefile(dir, "100.64.0.5"); err != nil {
		t.Fatalf("WriteCorefile returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "dns", "Corefile"))
	if err != nil {
		t.Fatalf("reading Corefile: %v", err)
	}
	if !strings.Contains(string(data), "100.64.0.5") {
		t.Errorf("Corefile does not contain bind IP; got:\n%s", string(data))
	}
}
