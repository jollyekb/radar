package server

import (
	"reflect"
	"testing"
)

// TestDefaultExecCommand covers the precedence rules for building the argv
// passed to the pod exec subresource:
//   - ?shell= override always wins and is passed verbatim as a single argv element
//   - --pod-shell-default fallback is used when no override is present
//   - the built-in defaultShellScript is used when neither is set
func TestDefaultExecCommand(t *testing.T) {
	tests := []struct {
		name     string
		override string
		fallback string
		want     []string
	}{
		{
			name:     "no override and no fallback uses built-in script",
			override: "",
			fallback: "",
			want:     []string{"sh", "-c", defaultShellScript},
		},
		{
			name:     "fallback configured, no override, wraps in sh -c",
			override: "",
			fallback: "exec zsh",
			want:     []string{"sh", "-c", "exec zsh"},
		},
		{
			name:     "explicit override without fallback is passed verbatim",
			override: "/bin/bash",
			fallback: "",
			want:     []string{"/bin/bash"},
		},
		{
			name:     "explicit override wins over configured fallback",
			override: "/bin/dash",
			fallback: "exec zsh",
			want:     []string{"/bin/dash"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultExecCommand(tc.override, tc.fallback)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("defaultExecCommand(%q, %q) = %v, want %v", tc.override, tc.fallback, got, tc.want)
			}
		})
	}
}

// TestDefaultShellScript documents the exact script content. Its role is to
// fail loudly if someone edits defaultShellScript without updating the
// verification checklist in the plan — the script is load-bearing for
// skyhook-io/radar#452.
//
// Note: earlier drafts used `exec bash || exec ash || exec sh`, but POSIX
// requires non-interactive shells to exit immediately when `exec` fails to
// find the target. That broke the fallback chain on images without bash.
// The current form detects each shell with `command -v` before execing,
// so exec only runs for commands that are known to exist.
func TestDefaultShellScript(t *testing.T) {
	const expected = "export TERM=xterm-256color; if command -v bash >/dev/null 2>&1; then exec bash -il; elif command -v ash >/dev/null 2>&1; then exec ash; else exec sh; fi"
	if defaultShellScript != expected {
		t.Errorf("defaultShellScript changed:\n  got:  %q\n  want: %q", defaultShellScript, expected)
	}
}
