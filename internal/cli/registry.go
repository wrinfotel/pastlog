package cli

import (
	"github.com/pastlog/pastlog/internal/adapters/claudecode"
	"github.com/pastlog/pastlog/internal/adapters/codex"
	"github.com/pastlog/pastlog/internal/agentlog"
)

// newRegistry builds the adapter set for one home directory.
func newRegistry(home string) *agentlog.Registry {
	reg := agentlog.NewRegistry()
	reg.Register(claudecode.New(home))
	reg.Register(codex.New(home))
	return reg
}
