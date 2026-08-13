//go:build windows

package record

import "os"

// raiseSignal is a no-op on Windows: there is no signal re-delivery to mirror
// the Unix 128+signal convention, so the caller falls back to an explicit
// exit after the abandoned state is written.
func raiseSignal(received os.Signal) bool {
	return false
}
