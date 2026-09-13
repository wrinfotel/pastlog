// Package version holds build metadata injected at link time via -ldflags:
//
//	-X github.com/pastlog/pastlog/internal/version.Version=v0.1.0
package version

// Build information, overridable via -ldflags -X.
var (
	Version = "0.0.0-dev"
	Commit  = "none"
	Date    = "unknown"
)
