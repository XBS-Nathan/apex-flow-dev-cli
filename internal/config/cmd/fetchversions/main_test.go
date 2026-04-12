package main

import (
	"testing"
)

func TestFilterPHP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "extracts major.minor from X.Y-fpm tags",
			tags: []string{"8.4-fpm", "8.3-fpm", "8.2-fpm"},
			want: []string{"8.4", "8.3", "8.2"},
		},
		{
			name: "ignores alpine variants",
			tags: []string{"8.4-fpm", "8.4-fpm-alpine", "8.3-fpm"},
			want: []string{"8.4", "8.3"},
		},
		{
			name: "ignores bare version without -fpm suffix",
			tags: []string{"8.4", "8.4-fpm"},
			want: []string{"8.4"},
		},
		{
			name: "ignores tag with no version prefix",
			tags: []string{"fpm", "latest", "8.4-fpm"},
			want: []string{"8.4"},
		},
		{
			name: "deduplicates versions",
			tags: []string{"8.4-fpm", "8.4-fpm", "8.3-fpm"},
			want: []string{"8.4", "8.3"},
		},
		{
			name: "sorts descending numerically",
			tags: []string{"8.1-fpm", "8.4-fpm", "8.2-fpm", "8.3-fpm"},
			want: []string{"8.4", "8.3", "8.2", "8.1"},
		},
		{
			name: "empty input returns nil",
			tags: []string{},
			want: nil,
		},
		{
			name: "no matching tags returns nil",
			tags: []string{"latest", "alpine", "8.4-fpm-alpine"},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterPHP(tt.tags)
			assertVersionsEqual(t, got, tt.want)
		})
	}
}

func TestFilterNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "extracts major from X-alpine tags",
			tags: []string{"22-alpine", "20-alpine", "18-alpine"},
			want: []string{"22", "20", "18"},
		},
		{
			name: "ignores alpine with distro suffix",
			tags: []string{"22-alpine", "22-alpine3.23", "20-alpine"},
			want: []string{"22", "20"},
		},
		{
			name: "ignores bare version without -alpine suffix",
			tags: []string{"22", "22-alpine"},
			want: []string{"22"},
		},
		{
			name: "ignores tag with no version prefix",
			tags: []string{"alpine", "latest", "22-alpine"},
			want: []string{"22"},
		},
		{
			name: "deduplicates versions",
			tags: []string{"22-alpine", "22-alpine", "20-alpine"},
			want: []string{"22", "20"},
		},
		{
			name: "sorts descending numerically",
			tags: []string{"18-alpine", "22-alpine", "20-alpine"},
			want: []string{"22", "20", "18"},
		},
		{
			name: "empty input returns nil",
			tags: []string{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterNode(tt.tags)
			assertVersionsEqual(t, got, tt.want)
		})
	}
}

func TestFilterMajorMinor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "extracts bare X.Y tags",
			tags: []string{"9.0", "8.4", "8.0"},
			want: []string{"9.0", "8.4", "8.0"},
		},
		{
			name: "ignores tags with suffix",
			tags: []string{"9.0", "9.0-oracle", "8.4-bookworm"},
			want: []string{"9.0"},
		},
		{
			name: "ignores non-version tags",
			tags: []string{"latest", "oracle", "9.0"},
			want: []string{"9.0"},
		},
		{
			name: "deduplicates versions",
			tags: []string{"9.0", "9.0", "8.4"},
			want: []string{"9.0", "8.4"},
		},
		{
			name: "sorts descending numerically",
			tags: []string{"8.0", "9.0", "8.4"},
			want: []string{"9.0", "8.4", "8.0"},
		},
		{
			name: "empty input returns nil",
			tags: []string{},
			want: nil,
		},
		{
			name: "ignores bare major version",
			tags: []string{"9", "9.0"},
			want: []string{"9.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterMajorMinor(tt.tags)
			assertVersionsEqual(t, got, tt.want)
		})
	}
}

func TestFilterMajor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "extracts bare major tags",
			tags: []string{"17", "16", "15"},
			want: []string{"17", "16", "15"},
		},
		{
			name: "ignores tags with suffix",
			tags: []string{"17", "17-trixie", "16-bookworm"},
			want: []string{"17"},
		},
		{
			name: "ignores non-version tags",
			tags: []string{"latest", "alpine", "17"},
			want: []string{"17"},
		},
		{
			name: "deduplicates versions",
			tags: []string{"17", "17", "16"},
			want: []string{"17", "16"},
		},
		{
			name: "sorts descending numerically",
			tags: []string{"9", "17", "13", "16"},
			want: []string{"17", "16", "13", "9"},
		},
		{
			name: "empty input returns nil",
			tags: []string{},
			want: nil,
		},
		{
			name: "ignores major.minor version",
			tags: []string{"9.0", "17"},
			want: []string{"17"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterMajor(tt.tags)
			assertVersionsEqual(t, got, tt.want)
		})
	}
}

func TestSortVersionsDesc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		versions []string
		want     []string
	}{
		{
			name:     "sorts major.minor descending",
			versions: []string{"8.1", "8.4", "8.2"},
			want:     []string{"8.4", "8.2", "8.1"},
		},
		{
			name:     "sorts multi-digit major descending",
			versions: []string{"9", "17", "13"},
			want:     []string{"17", "13", "9"},
		},
		{
			name:     "sorts across major versions",
			versions: []string{"8.0", "9.0", "8.4"},
			want:     []string{"9.0", "8.4", "8.0"},
		},
		{
			name:     "single element unchanged",
			versions: []string{"8.4"},
			want:     []string{"8.4"},
		},
		{
			name:     "empty slice unchanged",
			versions: []string{},
			want:     []string{},
		},
		{
			name:     "already sorted stays sorted",
			versions: []string{"17", "16", "15"},
			want:     []string{"17", "16", "15"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Copy to avoid mutating test data.
			input := make([]string, len(tt.versions))
			copy(input, tt.versions)
			sortVersionsDesc(input)
			assertVersionsEqual(t, input, tt.want)
		})
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want int // positive, negative, or zero
	}{
		{
			name: "8.4 > 8.3 (minor difference)",
			a:    "8.4",
			b:    "8.3",
			want: 1,
		},
		{
			name: "9.0 > 8.4 (major difference)",
			a:    "9.0",
			b:    "8.4",
			want: 1,
		},
		{
			name: "17 > 9 (multi-digit major)",
			a:    "17",
			b:    "9",
			want: 1,
		},
		{
			name: "8.3 < 8.4",
			a:    "8.3",
			b:    "8.4",
			want: -1,
		},
		{
			name: "8.4 == 8.4",
			a:    "8.4",
			b:    "8.4",
			want: 0,
		},
		{
			name: "17 == 17",
			a:    "17",
			b:    "17",
			want: 0,
		},
		{
			name: "9.0 > 9 (minor component defaults to 0)",
			a:    "9.0",
			b:    "9",
			want: 0,
		},
		{
			name: "9.1 > 9 (implicit minor 0)",
			a:    "9.1",
			b:    "9",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := compareVersions(tt.a, tt.b)
			switch {
			case tt.want > 0 && got <= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want positive", tt.a, tt.b, got)
			case tt.want < 0 && got >= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want negative", tt.a, tt.b, got)
			case tt.want == 0 && got != 0:
				t.Errorf("compareVersions(%q, %q) = %d, want 0", tt.a, tt.b, got)
			}
		})
	}
}

// assertVersionsEqual is a test helper that compares two string slices.
func assertVersionsEqual(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q (full result: %v)", i, got[i], want[i], got)
		}
	}
}
