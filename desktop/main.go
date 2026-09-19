// Command pastlog-desktop is the Wails bootstrap of pastlog Desktop: window
// config, the strict-CSP asset server, and the two bound services. All data
// behavior lives in internal/* and desktop/app — this file is glue only
// (TASK-DESKTOP.md §4.1).
//
// Untagged on purpose: the wails CLI's binding-generation pass builds this
// package without custom tags (R-D14). The frontend/dist embed target is
// kept non-empty by the committed .gitkeep placeholder.
package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wrinfotel/pastlog/desktop/app"
)

//go:embed all:frontend/dist
var assets embed.FS

// csp is the whole app's content-security policy (spec §2.2, §4.4):
// everything from the embedded assets, nothing remote, no inline script,
// no object/embed. Production CSS is real files, so style-src 'self' holds;
// under `wails dev` only the style allowance is relaxed for the dev
// server's injected <style> tags (R-D15) — script-src stays locked.
func csp() string {
	styles := "'self'"
	if devMode {
		styles = "'self' 'unsafe-inline'"
	}
	return "default-src 'self'; script-src 'self'; style-src " + styles + "; " +
		"img-src 'self' data:; font-src 'self'; connect-src 'self' ws://localhost:* http://localhost:*; " +
		"object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
}

// securityHeaders injects the CSP on every served asset.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp())
		next.ServeHTTP(w, r)
	})
}

// eventSink adapts desktop/app progress events to Wails runtime events.
type eventSink struct {
	ctx atomic.Pointer[context.Context]
}

func (s *eventSink) set(ctx context.Context) { s.ctx.Store(&ctx) }

// Progress implements app.EventSink.
func (s *eventSink) Progress(op string, scanned, hits int) {
	if c := s.ctx.Load(); c != nil {
		wailsruntime.EventsEmit(*c, "progress", map[string]any{"op": op, "scanned": scanned, "hits": hits})
	}
}

// guiBridge is the wails-aware companion service: OS dialogs and browser
// links that must not touch internal/*, kept out of desktop/app so the
// no-network audit can hold the glue to a stricter standard (R-D3).
type guiBridge struct {
	ctx atomic.Pointer[context.Context]
}

func (b *guiBridge) set(ctx context.Context) { b.ctx.Store(&ctx) }

// PickSavePath shows the OS save dialog; "" when cancelled.
func (b *guiBridge) PickSavePath(defaultName string) (string, error) {
	return wailsruntime.SaveFileDialog(*b.ctx.Load(), wailsruntime.SaveDialogOptions{
		Title:           "Export",
		DefaultFilename: defaultName,
	})
}

// OpenExternal opens an explicit user-clicked link in the system browser —
// never inside the app (spec §4.4).
func (b *guiBridge) OpenExternal(url string) {
	wailsruntime.BrowserOpenURL(*b.ctx.Load(), url)
}

func main() {
	sink := &eventSink{}
	bridge := &guiBridge{}
	service := app.New(app.WithSink(sink))
	if err := wails.Run(&options.App{
		Title:     "pastlog Desktop",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: securityHeaders,
		},
		OnStartup: func(ctx context.Context) {
			sink.set(ctx)
			bridge.set(ctx)
		},
		Bind: []interface{}{service, bridge},
	}); err != nil {
		log.Fatal(err)
	}
}
