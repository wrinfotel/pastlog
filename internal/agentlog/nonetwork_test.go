package agentlog

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoNetworkDeps is the spec §2.1 gate, verbatim: pastlog is 100% local,
// so the binary must not depend on any network code. It runs
// `go list -deps` over the whole module and fails, listing the offenders, if
// net/http, crypto/tls or any third-party HTTP client appears in the
// dependency graph. The module-qualified pattern resolves from any directory
// inside the module, so this test covers every package including cmd/pastlog.
func TestNoNetworkDeps(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH: cannot audit the dependency graph")
	}
	out, err := exec.Command(goBin, "list", "-deps", "github.com/wrinfotel/pastlog/...").CombinedOutput()
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
		t.Errorf("pastlog must not depend on network code; offending packages in `go list -deps ./...`:\n  %s\nremove them or replace them with stdlib non-network code",
			strings.Join(offenders, "\n  "))
	}
}
