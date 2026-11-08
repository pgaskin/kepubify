//go:build !scheduler.none

package internal

// parallelGoroutines is false for TinyGo builds without a scheduler, which
// don't support compiling go statements.
const parallelGoroutines = true
