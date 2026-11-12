package app

import (
	"slices"
	"testing"
)

func TestPOSIXShellCommandUsesLoginShell(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shell string
		want  []string
	}{
		{"user shell", "/bin/zsh", []string{"/bin/zsh", "-l"}},
		{"fallback", "", []string{"/bin/sh", "-l"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := posixShellCommand(tc.shell); !slices.Equal(got, tc.want) {
				t.Fatalf("posixShellCommand(%q) = %q, want %q", tc.shell, got, tc.want)
			}
		})
	}
}
