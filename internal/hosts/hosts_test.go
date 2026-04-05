package hosts

import (
	"os"
	"strings"
	"testing"
)

func TestEnsure_AddsEntry(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()

	if err := ensureEntry(f.Name(), "myproject.test"); err != nil {
		t.Fatalf("ensureEntry returned error: %v", err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}

	want := "127.0.0.1 myproject.test"
	if !strings.Contains(string(data), want) {
		t.Errorf("hosts file does not contain %q; got:\n%s", want, string(data))
	}
}

func TestEnsure_SkipsExistingEntry(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "hosts-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	entry := "127.0.0.1 myproject.test\n"
	if _, err := f.WriteString(entry); err != nil {
		t.Fatalf("write initial content: %v", err)
	}
	f.Close()

	// Call twice to ensure idempotency
	if err := ensureEntry(f.Name(), "myproject.test"); err != nil {
		t.Fatalf("ensureEntry returned error: %v", err)
	}
	if err := ensureEntry(f.Name(), "myproject.test"); err != nil {
		t.Fatalf("ensureEntry second call returned error: %v", err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}

	count := strings.Count(string(data), "127.0.0.1 myproject.test")
	if count != 1 {
		t.Errorf("expected entry to appear exactly once; got %d times in:\n%s", count, string(data))
	}
}

func TestIsWSL2(t *testing.T) {
	// Just verify it doesn't panic; result depends on environment
	_ = isWSL2()
}
