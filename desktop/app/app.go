// Package app holds pastlog Desktop's bound services: thin glue over the
// internal/* data layer. No business logic here — filter structs in, render
// JSON structs out (TASK-DESKTOP.md §4.1). This package must never import
// wails or any network package (enforced by TestNoNetworkDeps, ruling R-D3).
package app

// App is the service surface bound to the frontend.
type App struct{}

func New() *App { return &App{} }

// Ping exists only for the D0 spike; removed by T09.
func (a *App) Ping() string { return "pong" }
