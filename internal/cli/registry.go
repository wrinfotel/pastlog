package cli

import (
	"os"
	"path/filepath"

	"github.com/wrinfotel/pastlog/internal/adapters/antigravity"
	"github.com/wrinfotel/pastlog/internal/adapters/claudecode"
	"github.com/wrinfotel/pastlog/internal/adapters/codex"
	"github.com/wrinfotel/pastlog/internal/adapters/opencode"
	"github.com/wrinfotel/pastlog/internal/adapters/zcode"
	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/discovery"
)

// ResolveRegistry resolves storage for both the CLI and Desktop. A home
// chosen by the user takes precedence over agent-specific auto-discovery.
func ResolveRegistry(override string) (string, *agentlog.Registry, error) {
	home, err := discovery.Home(override)
	if err != nil {
		return "", nil, err
	}
	if override != "" {
		return home, NewRegistry(home), nil
	}
	claude, codexAdapter := claudecode.New(home), codex.New(home)
	if config := os.Getenv("CLAUDE_CONFIG_DIR"); config != "" {
		claude = claudecode.NewProjectsDir(filepath.Join(config, "projects"))
	}
	if config := os.Getenv("CODEX_HOME"); config != "" {
		codexAdapter = codex.NewCodexHome(config)
	}
	return home, newRegistry(home, claude, codexAdapter), nil
}

// NewRegistry builds the adapter set for an explicit home directory.
func NewRegistry(home string) *agentlog.Registry {
	return newRegistry(home, claudecode.New(home), codex.New(home))
}

func newRegistry(home string, claude *claudecode.Adapter, codexAdapter *codex.Adapter) *agentlog.Registry {
	reg := agentlog.NewRegistry()
	reg.Register(claude)
	reg.Register(codexAdapter)
	reg.Register(antigravity.New(home))
	reg.Register(opencode.New(home))
	reg.Register(zcode.New(home))
	return reg
}
