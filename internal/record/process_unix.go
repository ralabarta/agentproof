//go:build !windows

package record

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type processSignalForwarding uint8

const (
	processSignalForwarded processSignalForwarding = iota
	processSignalNotRunning
	processSignalUnsupported
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func forwardProcessSignal(process *os.Process, received os.Signal) processSignalForwarding {
	if process == nil {
		return processSignalNotRunning
	}
	sig, ok := received.(syscall.Signal)
	if !ok {
		return processSignalUnsupported
	}
	// os.Process marks itself done as part of Wait before Wait returns. Check
	// that state before addressing the process group by numeric ID, so a child
	// reaped just before lifecycle ownership is cleared cannot target a reused
	// PID/PGID.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return processSignalNotRunning
		}
		return processSignalUnsupported
	}
	if err := syscall.Kill(-process.Pid, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return processSignalNotRunning
		}
		return processSignalUnsupported
	}
	return processSignalForwarded
}
