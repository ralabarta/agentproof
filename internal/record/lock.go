package record

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
	"github.com/ralabarta/agentproof/internal/config"
)

const (
	recordLockMetadataVersion = 1
	recordLockMetadataMaxSize = 64 << 10
)

var errRecordLockContended = errors.New("record lock is held")

type recordLockMetadata struct {
	Version    int       `json:"version"`
	OwnerID    string    `json:"ownerID"`
	RunID      string    `json:"runID"`
	PID        int       `json:"pid"`
	AcquiredAt time.Time `json:"acquiredAt"`
}

type heldRecordLock struct {
	mu       sync.Mutex
	handle   platformLockHandle
	released bool
}

// LockMetadata identifies the valid metadata owner of a held record lock.
type LockMetadata struct {
	RunID string
}

// LockStatus reports kernel ownership and any valid owner metadata.
type LockStatus struct {
	Active   bool
	Metadata *LockMetadata
}

func acquireRecordLock(root, runID string) (*heldRecordLock, error) {
	if err := validateRecordRoot(filepath.Join(root, config.DirName)); err != nil {
		return nil, err
	}
	ownerID, err := newRecordLockOwnerID()
	if err != nil {
		return nil, err
	}
	handle, err := openRecordLock(recordLockPath(root))
	if err != nil {
		return nil, err
	}
	if err := lockRecordFile(handle); err != nil {
		if errors.Is(err, errRecordLockContended) {
			contentionErr := recordLockContentionError(handle)
			_ = closeRecordFile(handle)
			return nil, contentionErr
		}
		_ = closeRecordFile(handle)
		return nil, err
	}

	metadata := recordLockMetadata{
		Version:    recordLockMetadataVersion,
		OwnerID:    ownerID,
		RunID:      runID,
		PID:        os.Getpid(),
		AcquiredAt: time.Now().UTC(),
	}
	if err := validateRecordLockMetadata(metadata); err != nil {
		_ = unlockRecordFile(handle)
		_ = closeRecordFile(handle)
		return nil, err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		_ = unlockRecordFile(handle)
		_ = closeRecordFile(handle)
		return nil, err
	}
	if err := validateRecordRoot(filepath.Join(root, config.DirName)); err != nil {
		_ = unlockRecordFile(handle)
		_ = closeRecordFile(handle)
		return nil, err
	}
	if err := writeRecordLockFile(handle, append(data, '\n')); err != nil {
		_ = unlockRecordFile(handle)
		_ = closeRecordFile(handle)
		return nil, err
	}
	return &heldRecordLock{handle: handle}, nil
}

func (l *heldRecordLock) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	l.released = true
	unlockErr := unlockRecordFile(l.handle)
	closeErr := closeRecordFile(l.handle)
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func statusRecordLock(root string) (LockStatus, error) {
	return RecordLockStatus(root)
}

// RecordLockStatus probes the kernel lock without acquiring ownership.
func RecordLockStatus(root string) (LockStatus, error) {
	if err := validateRecordRoot(filepath.Join(root, config.DirName)); err != nil {
		return LockStatus{}, err
	}
	handle, err := openRecordLock(recordLockPath(root))
	if err != nil {
		return LockStatus{}, err
	}
	defer closeRecordFile(handle)

	lockErr := lockRecordFile(handle)
	if lockErr == nil {
		if err := unlockRecordFile(handle); err != nil {
			return LockStatus{}, err
		}
		return LockStatus{}, nil
	}
	if !errors.Is(lockErr, errRecordLockContended) {
		return LockStatus{}, lockErr
	}
	data, readErr := readRecordLockFile(handle, recordLockMetadataMaxSize)
	if readErr != nil {
		return LockStatus{Active: true}, nil
	}
	metadata, parseErr := parseRecordLockMetadata(data)
	if parseErr != nil {
		return LockStatus{Active: true}, nil
	}
	return LockStatus{
		Active:   true,
		Metadata: &LockMetadata{RunID: metadata.RunID},
	}, nil
}

func recordLockContentionError(handle platformLockHandle) error {
	data, err := readRecordLockFile(handle, recordLockMetadataMaxSize)
	if err == nil {
		if metadata, parseErr := parseRecordLockMetadata(data); parseErr == nil {
			return fmt.Errorf("%w: another agentproof record is already running (pid %d)", apperr.ErrUsage, metadata.PID)
		}
	}
	return fmt.Errorf("%w: another agentproof record is already running", apperr.ErrUsage)
}

func parseRecordLockMetadata(data []byte) (*recordLockMetadata, error) {
	if len(data) > recordLockMetadataMaxSize {
		return nil, fmt.Errorf("record lock metadata exceeds %d bytes", recordLockMetadataMaxSize)
	}
	var metadata recordLockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	if err := validateRecordLockMetadata(metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func validateRecordLockMetadata(metadata recordLockMetadata) error {
	if metadata.Version != recordLockMetadataVersion {
		return fmt.Errorf("unsupported record lock metadata version %d", metadata.Version)
	}
	if len(metadata.OwnerID) != 32 || strings.ToLower(metadata.OwnerID) != metadata.OwnerID {
		return errors.New("invalid record lock owner ID")
	}
	if _, err := hex.DecodeString(metadata.OwnerID); err != nil {
		return errors.New("invalid record lock owner ID")
	}
	if strings.TrimSpace(metadata.RunID) == "" {
		return errors.New("empty record lock run ID")
	}
	if metadata.PID <= 0 {
		return errors.New("invalid record lock PID")
	}
	if metadata.AcquiredAt.IsZero() {
		return errors.New("invalid record lock acquisition time")
	}
	_, offset := metadata.AcquiredAt.Zone()
	if offset != 0 {
		return errors.New("record lock acquisition time is not UTC")
	}
	return nil
}

func newRecordLockOwnerID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func recordLockPath(root string) string {
	return filepath.Join(root, config.DirName, ".record.lock")
}
