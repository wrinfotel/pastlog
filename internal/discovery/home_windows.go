//go:build windows

package discovery

import "os"

func homeFromEnv() string { return os.Getenv("USERPROFILE") }
