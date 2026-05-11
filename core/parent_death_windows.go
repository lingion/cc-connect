//go:build windows

// Package core — parent_death.go stub for Windows.
// Windows does not have prctl or kqueue equivalents for parent death detection.
// The safety net relies on EOF handling in the subprocess itself.
package core

import (
	"os/exec"
)

// SetParentDeathSignal is a no-op on Windows. There is no kernel-level
// mechanism to signal a child when the parent exits.
func SetParentDeathSignal(cmd *exec.Cmd) error {
	return nil
}

// MonitorParentDeath is a no-op on Windows. Returns an empty cleanup function.
func MonitorParentDeath(onParentDeath func()) func() {
	return func() {}
}