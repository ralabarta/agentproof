//go:build linux || darwin || freebsd

package record

import (
	"fmt"
	"os"
)

func validateRecordRoot(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("record root is a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("record root is not a directory")
	}
	return nil
}
