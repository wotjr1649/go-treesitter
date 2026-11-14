package main

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		pinned, latest string
		want           bool
	}{
		{"v0.27.2", "v0.27.2", false},
		{"v0.27.2", "v0.27.3", true},
		{"v0.27.2", "v0.28.0", true},
		{"v0.27.2", "v0.9.0", false},
		{"v0.27.2", "v1.0.0", true},
		{"34.0", "wasi-sdk-34", false},
		{"34.0", "wasi-sdk-35", true},
		{"34.0", "wasi-sdk-34.1", true},
		{"133", "version_133", false},
		{"133", "version_134", true},
		{"133", "version_99", false},
		{"v0.4.21", "v0.4.21", false},
		{"v0.4.21", "v0.4.100", true},
	} {
		if got := newer(c.pinned, c.latest); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.pinned, c.latest, got, c.want)
		}
	}
}
