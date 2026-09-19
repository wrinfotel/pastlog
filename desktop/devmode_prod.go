//go:build !dev

package main

// devMode is false in production builds: the strict CSP applies verbatim.
const devMode = false
