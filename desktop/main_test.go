// CSP middleware test — runs only with the desktop tag where frontend/dist
// exists (after `wails build`), i.e. in the CI desktop job and locally.
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersInjectCSP(t *testing.T) {
	called := false
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("the wrapped handler must run")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header missing")
	}
	for _, want := range []string{"default-src 'self'", "script-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP must contain %q, got %q", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-inline\"") || strings.Contains(csp, "script-src 'self' unsafe-inline") {
		t.Errorf("script-src must never allow inline script: %q", csp)
	}
	if devMode {
		// R-D15: dev relaxes style-src only.
		if !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") {
			t.Errorf("dev CSP must allow injected styles, got %q", csp)
		}
	} else if strings.Contains(csp, "unsafe-inline") {
		t.Errorf("production CSP must be strict (no unsafe-inline), got %q", csp)
	}
}
