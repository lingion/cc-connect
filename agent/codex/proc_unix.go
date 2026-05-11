//go:build unix

package codex

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"github.com/chenhg5/cc-connect/core"
)

func prepareCmdForKill(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true

	// Set parent death signal so subprocess receives SIGTERM when cc-connect dies.
	// This prevents zombie CPU spin-loops on unexpected parent termination.
	// Note: core.SetParentDeathSignal may set Pdeathsig, but Setpgid is kept
	// for proper process group killing via forceKillCmd.
	_ = core.SetParentDeathSignal(cmd)
}

func forceKillCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}