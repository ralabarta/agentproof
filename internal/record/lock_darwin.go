//go:build darwin

package record

import (
	"errors"
	"fmt"
	"syscall"
)

type platformLockHandle int

func openRecordLock(path string) (platformLockHandle, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return -1, err
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = syscall.Close(fd)
		return -1, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		_ = syscall.Close(fd)
		return -1, fmt.Errorf("record lock path is not a regular file")
	}
	return platformLockHandle(fd), nil
}

func lockRecordFile(handle platformLockHandle) error {
	err := syscall.Flock(int(handle), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errRecordLockContended
	}
	return err
}

func unlockRecordFile(handle platformLockHandle) error {
	return syscall.Flock(int(handle), syscall.LOCK_UN)
}

func closeRecordFile(handle platformLockHandle) error {
	return syscall.Close(int(handle))
}

func readRecordLockFile(handle platformLockHandle, limit int) ([]byte, error) {
	if _, err := syscall.Seek(int(handle), 0, 0); err != nil {
		return nil, err
	}
	data := make([]byte, limit+1)
	n, err := syscall.Read(int(handle), data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func writeRecordLockFile(handle platformLockHandle, data []byte) error {
	fd := int(handle)
	if err := syscall.Ftruncate(fd, 0); err != nil {
		return err
	}
	if _, err := syscall.Seek(fd, 0, 0); err != nil {
		return err
	}
	for len(data) > 0 {
		n, err := syscall.Write(fd, data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return syscall.Fsync(fd)
}
