package server

import (
	"reflect"
	"strings"
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

// TestDefaultShellScript is a tripwire that pins the exact script content.
// The script is load-bearing for skyhook-io/radar#452 — any edit should be
// manually re-verified against the scenarios documented in the PR that
// introduced it (bash present, ash-only, sh-only, --pod-shell-default
// override, multi-container pod). If you're here because this test failed,
// update `expected` AND re-run those scenarios before merging.
//
// Earlier drafts used `exec bash || exec ash || exec sh`, but POSIX requires
// non-interactive shells to exit immediately when `exec` fails to find the
// target — so that cascade died with exit 127 on the first missing shell.
// The current form detects each shell with `command -v` before execing, so
// exec only runs for commands that are known to exist.
func TestDefaultShellScript(t *testing.T) {
	const expected = "export TERM=xterm-256color; if command -v bash >/dev/null 2>&1; then exec bash -il; elif command -v ash >/dev/null 2>&1; then exec ash; else exec sh; fi"
	if defaultShellScript != expected {
		t.Errorf("defaultShellScript changed:\n  got:  %q\n  want: %q", defaultShellScript, expected)
	}

	// Behavioral asserts — pin the load-bearing properties so a future edit
	// that drops one of them fails with a specific message, not just a
	// string-diff. These would have caught the `exec bash || exec ash` draft
	// that broke live in alpine during this PR's development.
	if !strings.Contains(defaultShellScript, "command -v bash") {
		t.Error("defaultShellScript must detect bash with `command -v` before exec'ing — naive `exec bash || ...` fails under POSIX because non-interactive shells exit on exec-not-found")
	}
	if !strings.Contains(defaultShellScript, "bash -il") {
		t.Error("defaultShellScript must run bash as `-il` (interactive login) so it sources the image's startup files and picks up a PS1 with \\w — that's the PWD-in-prompt fix from skyhook-io/radar#452")
	}
}
