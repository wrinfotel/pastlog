package cli

import (
	"github.com/pastlog/pastlog/internal/adapters/claudecode"
	"github.com/pastlog/pastlog/internal/adapters/codex"
	"github.com/pastlog/pastlog/internal/adapters/geminicli"
	"github.com/pastlog/pastlog/internal/adapters/opencode"
	"github.com/pastlog/pastlog/internal/agentlog"
)

// newRegistry builds the adapter set for one home directory, in spec §4
// order (P0 first): claude-code, codex, gemini-cli, opencode.
func newRegistry(home string) *agentlog.Registry {
	reg := agentlog.NewRegistry()
	reg.Register(claudecode.New(home))
	reg.Register(codex.New(home))
	reg.Register(geminicli.New(home))
	reg.Register(opencode.New(home))
	return reg
}
