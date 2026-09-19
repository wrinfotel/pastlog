package discovery

import (
	"os"
	"path/filepath"
)

// ZCodeDir resolves the ZCode storage directory: the directory holding
// db.sqlite. ZCode is a Node app with its own fixed home on every OS, so the
// only candidate is <home>/.zcode/cli/db (observed layout; see
// internal/adapters/zcode/SCHEMA.md).
//
// An empty home or a non-directory candidate yields "" — absence of ZCode
// storage is not an error (spec §8). Tests override the location via the
// --home flag.
func ZCodeDir(home string) string {
	if home == "" {
		return ""
	}
	dir := filepath.Join(home, ".zcode", "cli", "db")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}
