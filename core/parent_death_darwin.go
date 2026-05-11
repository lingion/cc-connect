//go:build darwin

// Package core — parent_death_darwin.go provides parent process death detection
// for spawned subprocesses on macOS using kqueue with NOTE_EXIT.
//
// When the parent (cc-connect) dies unexpectedly, child processes (MCP servers,
// agent CLIs) should exit cleanly instead of entering a busy loop on EOF stdin.
//
// macOS does not have prctl(PR_SET_PDEATHSIG), so we use kqueue to monitor
// the parent process PID and trigger cleanup when it exits.
package core

import (
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

// SetParentDeathSignal is a no-op on macOS. There is no kernel-level mechanism
// like prctl(PR_SET_PDEATHSIG). Use MonitorParentDeath instead.
func SetParentDeathSignal(cmd *exec.Cmd) error {
	// macOS doesn't support Pdeathsig in SysProcAttr.
	// The parent death monitoring is done via kqueue in MonitorParentDeath.
	return nil
}

// MonitorParentDeath starts a goroutine that monitors the parent process
// and calls onParentDeath when the parent exits. This is the macOS approach
// since macOS doesn't have prctl(PR_SET_PDEATHSIG).
//
// Returns a function to stop monitoring. Caller should invoke it when
// the subprocess exits cleanly.
//
// Note: This only works if the parent PID doesn't change. If cc-connect
// daemon restarts (execve), the subprocess should detect EOF and exit
// before MonitorParentDeath becomes relevant.
func MonitorParentDeath(onParentDeath func()) func() {
	ppid := os.Getppid()
	stopCh := make(chan struct{})

	go func() {
		kq, err := syscall.Kqueue()
		if err != nil {
			slog.Debug("parent_death: kqueue create failed", "error", err)
			return
		}
		defer syscall.Close(kq)

		// Monitor parent process for exit.
		ev := syscall.Kevent_t{
			Ident:  uint64(ppid),
			Filter: syscall.EVFILT_PROC,
			Flags:  syscall.EV_ADD,
			Fflags: syscall.NOTE_EXIT,
		}

		// Register the event.
		_, err = syscall.Kevent(kq, []syscall.Kevent_t{ev}, nil, nil)
		if err != nil {
			slog.Debug("parent_death: kqueue register failed", "error", err, "ppid", ppid)
			return
		}

		// Wait for parent exit or stop signal.
		for {
			select {
			case <-stopCh:
				return
			default:
				// Poll with small timeout to interleave with stopCh check.
				ts := syscall.Timespec{Sec: 1, Nsec: 0}
				events := make([]syscall.Kevent_t, 1)
				n, err := syscall.Kevent(kq, nil, events, &ts)
				if err != nil {
					slog.Debug("parent_death: kqueue wait error", "error", err)
					return
				}
				if n > 0 && (events[0].Flags&syscall.EV_EOF != 0 || events[0].Fflags&syscall.NOTE_EXIT != 0) {
					slog.Info("parent_death: parent exited, invoking callback")
					onParentDeath()
					return
				}
			}
		}
	}()

	return func() {
		close(stopCh)
	}
}