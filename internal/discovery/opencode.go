package discovery

import (
	"os"
	"path/filepath"
	"runtime"
)

// OpenCodeDir resolves the OpenCode storage root: the directory holding
// opencode.db (M3 controller ruling, replacing the deferred M1 XDG work).
//
// Candidates, first existing one wins:
//
//	windows: %LOCALAPPDATA%/opencode, then <home>/.local/share/opencode
//	         (real OpenCode installs may resolve XDG themselves even on
//	         Windows, so the unix-style path stays a candidate)
//	unix:    $XDG_DATA_HOME/opencode (when set), then <home>/.local/share/opencode
//
// An empty or non-directory candidate is skipped. Returns "" when no
// candidate exists — absence of OpenCode storage is not an error (spec §8).
// Tests override the data dir via LOCALAPPDATA (Windows) / XDG_DATA_HOME (unix).
func OpenCodeDir(home string) string {
	for _, dir := range openCodeDirCandidates(home) {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// openCodeDirCandidates lists the storage-root candidates in priority order.
func openCodeDirCandidates(home string) []string {
	var out []string
	if data := dataHomeEnv(); data != "" {
		out = append(out, filepath.Join(data, "opencode"))
	}
	if home != "" {
		out = append(out, filepath.Join(home, ".local", "share", "opencode"))
	}
	return out
}

// dataHomeEnv returns the per-OS XDG data-dir override: %LOCALAPPDATA% on
// Windows, $XDG_DATA_HOME on unix.
func dataHomeEnv() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("LOCALAPPDATA")
	}
	return os.Getenv("XDG_DATA_HOME")
}
