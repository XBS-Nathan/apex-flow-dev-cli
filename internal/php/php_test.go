package php

import "testing"

func TestFPMSocket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    string
	}{
		{"8.2", "/run/php/php8.2-fpm.sock"},
		{"8.3", "/run/php/php8.3-fpm.sock"},
		{"7.4", "/run/php/php7.4-fpm.sock"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			got := FPMSocket(tt.version)
			if got != tt.want {
				t.Errorf("FPMSocket(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestFPMServiceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		version string
		want    string
	}{
		{"8.2", "php8.2-fpm"},
		{"8.3", "php8.3-fpm"},
		{"7.4", "php7.4-fpm"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			t.Parallel()
			got := FPMServiceName(tt.version)
			if got != tt.want {
				t.Errorf("FPMServiceName(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}
