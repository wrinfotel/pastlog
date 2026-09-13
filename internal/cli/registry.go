package cli

import (
	"github.com/pastlog/pastlog/internal/adapters/claudecode"
	"github.com/pastlog/pastlog/internal/agentlog"
)

// newRegistry builds the adapter set for one home directory. M1 registers
// only the claude-code adapter; later milestones extend this list.
func newRegistry(home string) *agentlog.Registry {
	reg := agentlog.NewRegistry()
	reg.Register(claudecode.New(home))
	return reg
}
