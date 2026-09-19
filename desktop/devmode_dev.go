//go:build dev

package main

// devMode is true under `wails dev` (the CLI builds with the dev tag). The
// dev server injects styles via <style> tags, which needs style-src
// 'unsafe-inline'; the production build keeps the strict CSP (R-D15).
const devMode = true
