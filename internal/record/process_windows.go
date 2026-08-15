//go:build windows

package record

import (
	"os"
	"os/exec"
)

type processSignalForwarding uint8

const (
	processSignalForwarded processSignalForwarding = iota
	processSignalNotRunning
	processSignalUnsupported
)

// configureProcess intentionally leaves the child in the wrapper's process
// context. Go's standard library cannot forward console control events to an
// isolated group, so isolation would orphan the child when the wrapper exits.
// Windows signal handling is explicitly best-effort.
func configureProcess(_ *exec.Cmd) {}

// The standard library cannot deliver Ctrl+C or Ctrl+Break to a selected
// process group on Windows. Report that limitation instead of claiming
// Unix-equivalent forwarding.
func forwardProcessSignal(process *os.Process, received os.Signal) processSignalForwarding {
	if process == nil {
		return processSignalNotRunning
	}
	return processSignalUnsupported
}
