//go:build linux

// Package core — parent_death_linux.go provides parent process death detection
// for spawned subprocesses on Linux using prctl(PR_SET_PDEATHSIG).
//
// When the parent (cc-connect) dies unexpectedly, child processes (MCP servers,
// agent CLIs) should exit cleanly instead of entering a busy loop on EOF stdin.
//
// This is a safety net that complements EOF handling and heartbeat mechanisms.
// It ensures that even if cc-connect crashes or is force-killed, spawned
// subprocesses don't become zombie CPU spinners.
package core

import (
	"log/slog"
	"os/exec"
	"syscall"
)

// SetParentDeathSignal configures cmd to receive SIGTERM when the parent
// process (cc-connect) exits. This prevents spawned subprocesses from
// becoming zombie CPU spinners when stdin EOF is not properly handled.
//
// On Linux: uses prctl(PR_SET_PDEATHSIG) via SysProcAttr.
func SetParentDeathSignal(cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// When parent dies, kernel sends SIGTERM to child.
	cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
	slog.Debug("parent_death: set PDEATHSIG=SIGTERM for subprocess")
	return nil
}

// MonitorParentDeath is not needed on Linux since prctl handles parent death
// detection kernel-side. Returns an empty cleanup function.
func MonitorParentDeath(onParentDeath func()) func() {
	return func() {}
}