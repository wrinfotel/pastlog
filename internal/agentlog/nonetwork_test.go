package agentlog

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoNetworkDeps is the spec §2.1 gate, verbatim: pastlog is 100% local,
// so the data path must not depend on any network code. It runs
// `go list -deps` over the audited packages (noNetworkAuditPatterns) and
// fails, listing the offenders, if net/http, crypto/tls or any third-party
// HTTP client appears in the dependency graph. Since the desktop app joined
// the module (TASK-DESKTOP.md), the audit is scoped by ruling R-D3: the CLI,
// the whole data layer and the desktop service glue stay network-free; the
// wails webview shell (desktop main) is the one deliberate exclusion — it
// serves its local assets over net/http by nature, and everything it binds
// (desktop/app) is audited here instead. The frontend side of the same
// guarantee is the bundle audit (desktop/frontend/scripts/audit-bundle.mjs).
func TestNoNetworkDeps(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH: cannot audit the dependency graph")
	}
	args := append([]string{"list", "-deps"}, noNetworkAuditPatterns()...)
	out, err := exec.Command(goBin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, out)
	}

	deps := strings.Fields(string(out))
	var offenders []string
	for _, dep := range deps {
		switch {
		case dep == "crypto/tls":
			offenders = append(offenders, dep+"  (TLS stack — spec §2.1 forbids network code)")
		case strings.HasPrefix(dep, "net/http"):
			offenders = append(offenders, dep+"  (HTTP code — spec §2.1 forbids network code)")
		case strings.Contains(strings.SplitN(dep, "/", 2)[0], ".") && strings.Contains(dep, "http"):
			// first path segment carries a dot → third-party module; any
			// http-named package there is a third-party HTTP client
			offenders = append(offenders, dep+"  (third-party HTTP client — spec §2.1)")
		}
	}
	if len(offenders) > 0 {
		t.Errorf("pastlog data path must not depend on network code; offending packages:\n  %s\nremove them or replace them with stdlib non-network code",
			strings.Join(offenders, "\n  "))
	}
}

// noNetworkAuditPatterns lists the module packages whose dependency graph
// must stay free of network code: the CLI, the whole data layer, and the
// desktop service glue. The desktop main package is excluded on purpose
// (see TestNoNetworkDeps); a corollary rule falls out of including
// desktop/app here: the glue may never import wails — main.go is the only
// wails-aware Go file (ruling R-D3).
func noNetworkAuditPatterns() []string {
	return []string{
		"github.com/wrinfotel/pastlog/cmd/...",
		"github.com/wrinfotel/pastlog/internal/...",
		"github.com/wrinfotel/pastlog/desktop/app",
	}
}

// TestNoNetworkDepsCoversDesktopGlue pins the audit's scope (R-D3): the
// patterns audited by TestNoNetworkDeps must include the desktop service
// package, so the GUI glue can never smuggle network code into the data
// path behind the desktop-main exclusion.
func TestNoNetworkDepsCoversDesktopGlue(t *testing.T) {
	for _, pattern := range noNetworkAuditPatterns() {
		if pattern == "github.com/wrinfotel/pastlog/desktop/app" {
			return
		}
	}
	t.Error("audit must cover desktop/app (R-D3): glue may not import wails or any network package")
}
