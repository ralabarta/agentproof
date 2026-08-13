//go:build !windows

package record

import (
	"os"
	"os/signal"
	"syscall"
)

// raiseSignal restores the default disposition for the received signal and
// re-delivers it to this process, so a shell observes the conventional
// 128+signal status instead of an arbitrary exit code. It reports whether the
// process is expected to die from the signal.
func raiseSignal(received os.Signal) bool {
	sig, ok := received.(syscall.Signal)
	if !ok {
		return false
	}
	signal.Reset(os.Interrupt, syscall.SIGTERM)
	return syscall.Kill(os.Getpid(), sig) == nil
}
