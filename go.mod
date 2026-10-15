module iacpack

// Go 1.27.1 is the pinned language/toolchain baseline (TZ.md / P00).
// A separate `toolchain` directive is intentionally omitted: go mod tidy
// removes it when it equals the go line; GOTOOLCHAIN=go1.27.1 is also set
// in the Makefile and CI for explicit developer/CI selection.
//
// This module intentionally has no third-party require directives so runnable
// source and the built binary do not embed foreign module/URL import paths.
go 1.27.1
