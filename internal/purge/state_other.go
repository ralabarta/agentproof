//go:build !linux && !darwin && !freebsd && !windows

package purge

import (
	"errors"
	"os"
)

func openStateFile(string) (*os.File, error) {
	return nil, errors.New("safe state reads are unsupported on this platform")
}
