// Package discovery resolves the user home directory used to locate agent
// storage: an explicit --home override wins; otherwise the per-OS home env
// var (USERPROFILE on Windows, HOME on unix) with os.UserHomeDir as fallback.
package discovery

import (
	"fmt"
	"os"
)

// Home returns the home directory to scan. A non-empty override must point
// at an existing directory. Errors are lowercase and actionable (spec §8).
func Home(override string) (string, error) {
	if override != "" {
		info, err := os.Stat(override)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("home directory not found: %s", override)
			}
			return "", fmt.Errorf("cannot access home directory %s: %v", override, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("home path is not a directory: %s", override)
		}
		return override, nil
	}

	dir := homeFromEnv()
	if dir == "" {
		var err error
		dir, err = os.UserHomeDir()
		if err != nil || dir == "" {
			return "", fmt.Errorf("cannot determine the home directory; pass --home <dir>")
		}
	}
	return dir, nil
}
