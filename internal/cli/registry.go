package cli

import (
	"github.com/wrinfotel/pastlog/internal/adapters/claudecode"
	"github.com/wrinfotel/pastlog/internal/adapters/codex"
	"github.com/wrinfotel/pastlog/internal/adapters/geminicli"
	"github.com/wrinfotel/pastlog/internal/adapters/opencode"
	"github.com/wrinfotel/pastlog/internal/adapters/zcode"
	"github.com/wrinfotel/pastlog/internal/agentlog"
)

// NewRegistry builds the adapter set for one home directory, in spec §4
// order (P0 first): claude-code, codex, gemini-cli, opencode, zcode. It is
// the single registration point shared by the CLI and the desktop app
// (ruling R-D7): both surfaces must always see the same adapter set.
func NewRegistry(home string) *agentlog.Registry {
	reg := agentlog.NewRegistry()
	reg.Register(claudecode.New(home))
	reg.Register(codex.New(home))
	reg.Register(geminicli.New(home))
	reg.Register(opencode.New(home))
	reg.Register(zcode.New(home))
	return reg
}
