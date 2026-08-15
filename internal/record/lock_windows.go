//go:build windows

package record

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	fileAttributeTagInfo    = 9
	lockFileExclusiveLock   = 0x00000002
	lockFileFailImmediately = 0x00000001
	errorLockViolation      = syscall.Errno(33)
	recordLockByteOffset    = recordLockMetadataMaxSize + 1
)

var (
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procGetFileInformationByHandleEx = kernel32.NewProc("GetFileInformationByHandleEx")
	procLockFileEx                   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx                 = kernel32.NewProc("UnlockFileEx")
)

type platformLockHandle syscall.Handle

type fileAttributeTagInformation struct {
	FileAttributes uint32
	ReparseTag     uint32
}

func validateRecordRoot(path string) error {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)

	fileType, err := syscall.GetFileType(handle)
	if err != nil {
		return err
	}
	if fileType != syscall.FILE_TYPE_DISK {
		return errors.New("record root is not a disk directory")
	}
	info, err := getFileAttributeTagInformation(handle)
	if err != nil {
		return err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("record root is a reparse point")
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return errors.New("record root is not a directory")
	}
	return nil
}

func openRecordLock(path string) (platformLockHandle, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return platformLockHandle(syscall.InvalidHandle), err
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return platformLockHandle(syscall.InvalidHandle), err
	}
	if err := validateRecordLockHandle(handle); err != nil {
		_ = syscall.CloseHandle(handle)
		return platformLockHandle(syscall.InvalidHandle), err
	}
	return platformLockHandle(handle), nil
}

func validateRecordLockHandle(handle syscall.Handle) error {
	fileType, err := syscall.GetFileType(handle)
	if err != nil {
		return err
	}
	if fileType != syscall.FILE_TYPE_DISK {
		return errors.New("record lock path is not a disk file")
	}
	info, err := getFileAttributeTagInformation(handle)
	if err != nil {
		return err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("record lock path is a reparse point")
	}
	return nil
}

func getFileAttributeTagInformation(handle syscall.Handle) (fileAttributeTagInformation, error) {
	var info fileAttributeTagInformation
	r1, _, callErr := procGetFileInformationByHandleEx.Call(
		uintptr(handle),
		fileAttributeTagInfo,
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r1 == 0 {
		return fileAttributeTagInformation{}, callErr
	}
	return info, nil
}

func lockRecordFile(handle platformLockHandle) error {
	var overlapped syscall.Overlapped
	// Reads include one oversize sentinel byte; lock the following byte so
	// contenders can still inspect diagnostic ownership metadata.
	overlapped.Offset = recordLockByteOffset
	r1, _, callErr := procLockFileEx.Call(
		uintptr(handle),
		lockFileExclusiveLock|lockFileFailImmediately,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 != 0 {
		return nil
	}
	if errors.Is(callErr, errorLockViolation) {
		return errRecordLockContended
	}
	return callErr
}

func unlockRecordFile(handle platformLockHandle) error {
	var overlapped syscall.Overlapped
	overlapped.Offset = recordLockByteOffset
	r1, _, callErr := procUnlockFileEx.Call(
		uintptr(handle),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		return callErr
	}
	return nil
}

func closeRecordFile(handle platformLockHandle) error {
	return syscall.CloseHandle(syscall.Handle(handle))
}

func readRecordLockFile(handle platformLockHandle, limit int) ([]byte, error) {
	if _, err := syscall.SetFilePointer(syscall.Handle(handle), 0, nil, 0); err != nil {
		return nil, err
	}
	data := make([]byte, limit+1)
	var n uint32
	if err := syscall.ReadFile(syscall.Handle(handle), data, &n, nil); err != nil {
		return nil, err
	}
	return data[:n], nil
}

func writeRecordLockFile(handle platformLockHandle, data []byte) error {
	windowsHandle := syscall.Handle(handle)
	if _, err := syscall.SetFilePointer(windowsHandle, 0, nil, 0); err != nil {
		return err
	}
	if err := syscall.SetEndOfFile(windowsHandle); err != nil {
		return err
	}
	for len(data) > 0 {
		var n uint32
		if err := syscall.WriteFile(windowsHandle, data, &n, nil); err != nil {
			return err
		}
		data = data[n:]
	}
	return syscall.FlushFileBuffers(windowsHandle)
}
