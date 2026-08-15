package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
	"github.com/ralabarta/agentproof/internal/config"
)

const recordLockHelperEnv = "AP_RECORD_LOCK_HELPER"

func acquireForTest(root, runID string) (*heldRecordLock, error) {
	return acquireRecordLock(root, runID)
}

func TestRecordLockMetadataContract(t *testing.T) {
	root := newRepo(t)
	lock, err := acquireForTest(root, "run-metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	var metadata struct {
		Version    int       `json:"version"`
		OwnerID    string    `json:"ownerID"`
		RunID      string    `json:"runID"`
		PID        int       `json:"pid"`
		AcquiredAt time.Time `json:"acquiredAt"`
	}
	data, err := os.ReadFile(filepath.Join(root, config.DirName, ".record.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("lock metadata must be JSON: %v (bytes %q)", err, data)
	}
	if metadata.Version != 1 {
		t.Fatalf("metadata version = %d, want 1", metadata.Version)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(metadata.OwnerID) {
		t.Fatalf("ownerID = %q, want 128-bit lowercase hexadecimal", metadata.OwnerID)
	}
	if metadata.RunID != "run-metadata" {
		t.Fatalf("runID = %q, want run-metadata", metadata.RunID)
	}
	if metadata.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", metadata.PID, os.Getpid())
	}
	if metadata.AcquiredAt.IsZero() || metadata.AcquiredAt.Location() != time.UTC {
		t.Fatalf("acquiredAt = %v, want a valid UTC timestamp", metadata.AcquiredAt)
	}
}

func TestRecordLockPersistsAfterRelease(t *testing.T) {
	root := newRepo(t)
	lock, err := acquireForTest(root, "run-persist")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(filepath.Join(root, config.DirName, ".record.lock"))
	if err != nil {
		t.Fatalf("released lock path must persist: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("released lock mode = %v, want regular file", info.Mode())
	}
}

func TestRecordLockStatusReportsOnlyKernelHeldOwner(t *testing.T) {
	root := newRepo(t)
	stale, err := acquireForTest(root, "run-stale")
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.Release(); err != nil {
		t.Fatal(err)
	}

	status, err := RecordLockStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Active || status.Metadata != nil {
		t.Fatalf("released lock status = %#v, want inactive without owner metadata", status)
	}

	control := t.TempDir()
	result := filepath.Join(control, "result")
	release := filepath.Join(control, "release")
	cmd := recordLockHelperCommand(root, "run-active", "hold", "", result, release)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	helper := monitorRecordLockHelper(cmd)
	t.Cleanup(func() {
		cleanupRecordLockHelper(t, helper, release)
	})
	if got := waitForRecordLockHelper(t, helper, result); got != "acquired" {
		t.Fatalf("lock helper result = %q, want acquired", got)
	}

	status, err = RecordLockStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Metadata == nil || status.Metadata.RunID != "run-active" {
		t.Fatalf("held lock status = %#v, want active owner run-active", status)
	}

	if err := os.WriteFile(filepath.Join(root, config.DirName, ".record.lock"), []byte("malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = RecordLockStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Metadata != nil {
		t.Fatalf("malformed held lock status = %#v, want active without owner metadata", status)
	}
}

func TestRecordLockRejectsFinalSymlink(t *testing.T) {
	root := newRepo(t)
	target := filepath.Join(root, "lock-target")
	original := []byte("do not overwrite")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, config.DirName, ".record.lock")
	if err := os.Symlink(target, lockPath); err != nil {
		if runtime.GOOS == "windows" && (errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "privilege")) {
			t.Skipf("creating symlinks requires Windows developer mode or privilege: %v", err)
		}
		t.Fatal(err)
	}

	lock, err := acquireForTest(root, "run-symlink")
	if err == nil {
		_ = lock.Release()
		t.Fatal("expected final lock-path symlink to be rejected")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("symlink target changed to %q, want %q", got, original)
	}
}

func TestRecordLockRejectsSymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	sentinelPath := filepath.Join(external, "sentinel")
	original := []byte("do not overwrite")
	if err := os.WriteFile(sentinelPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, config.DirName)); err != nil {
		if runtime.GOOS == "windows" && (errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "privilege")) {
			t.Skipf("creating a directory symlink requires Windows developer mode or privilege: %v", err)
		}
		t.Fatal(err)
	}

	lock, err := acquireForTest(root, "run-symlinked-root")
	if err == nil {
		_ = lock.Release()
		t.Fatal("expected symlinked record root to be rejected")
	}
	got, readErr := os.ReadFile(sentinelPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("external sentinel changed to %q, want %q", got, original)
	}
	if _, statErr := os.Lstat(filepath.Join(external, ".record.lock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("external lock path must not be created, got %v", statErr)
	}
}

func TestRecordLockCrashAutoReleases(t *testing.T) {
	root := newRepo(t)
	result := filepath.Join(t.TempDir(), "crash-result")
	cmd := recordLockHelperCommand(root, "run-crash", "crash", "", result, "")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	helper := monitorRecordLockHelper(cmd)
	if got := waitForRecordLockHelper(t, helper, result); got != "acquired" {
		t.Fatalf("crash helper result = %q, want acquired", got)
	}
	select {
	case <-helper.done:
	case <-time.After(2 * time.Second):
		killAndReapRecordLockHelper(t, helper)
		t.Fatal("timed out waiting for crash helper to exit")
	}
	if helper.err == nil {
		t.Fatal("crash helper unexpectedly exited successfully")
	}

	lock, err := acquireForTest(root, "run-after-crash")
	if err != nil {
		t.Fatalf("kernel lock must auto-release after holder crash: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordLockConcurrentAcquisitionHasSingleWinner(t *testing.T) {
	root := newRepo(t)
	control := t.TempDir()
	start := filepath.Join(control, "start")
	release := filepath.Join(control, "release")
	const contenders = 12
	results := make([]string, contenders)
	commands := make([]*exec.Cmd, contenders)
	helpers := make([]*recordLockHelperProcess, contenders)
	for i := range commands {
		results[i] = filepath.Join(control, fmt.Sprintf("result-%d", i))
		commands[i] = recordLockHelperCommand(root, fmt.Sprintf("run-%d", i), "hold", start, results[i], release)
	}
	for i, cmd := range commands {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		helpers[i] = monitorRecordLockHelper(cmd)
		helper := helpers[i]
		t.Cleanup(func() {
			cleanupRecordLockHelper(t, helper, release)
		})
	}
	if err := os.WriteFile(start, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	values := make([]string, contenders)
	for i, result := range results {
		values[i] = waitForRecordLockHelper(t, helpers[i], result)
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, helper := range helpers {
		select {
		case <-helper.done:
			if helper.err != nil {
				t.Fatalf("lock helper failed: %v", helper.err)
			}
		case <-time.After(2 * time.Second):
			killAndReapRecordLockHelper(t, helper)
			t.Fatal("timed out waiting for lock helper to exit")
		}
	}

	acquired := 0
	usageFailures := 0
	for _, value := range values {
		switch {
		case value == "acquired":
			acquired++
		case strings.HasPrefix(value, "usage:another agentproof record"):
			usageFailures++
		default:
			t.Fatalf("unexpected helper result %q", value)
		}
	}
	if acquired != 1 || usageFailures != contenders-1 {
		t.Fatalf("results: acquired=%d usage failures=%d, want one winner and %d usage failures", acquired, usageFailures, contenders-1)
	}
}

func TestRecordLockReleaseCannotUnlockAnotherOwner(t *testing.T) {
	root := newRepo(t)
	first, err := acquireForTest(root, "run-first")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireForTest(root, "run-second")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()

	if err := first.Release(); err != nil {
		t.Fatalf("repeated release must be harmless: %v", err)
	}
	third, err := acquireForTest(root, "run-third")
	if err == nil {
		_ = third.Release()
		t.Fatal("releasing an old owner unlocked the current owner")
	}
	if !apperr.IsUsage(err) {
		t.Fatalf("third acquisition error = %v, want usage-class contention", err)
	}
}

func TestRecordLockProcessHelper(t *testing.T) {
	if os.Getenv(recordLockHelperEnv) != "1" {
		return
	}
	start := os.Getenv("AP_RECORD_LOCK_START")
	if start != "" {
		waitForPath(start)
	}
	lock, err := acquireForTest(os.Getenv("AP_RECORD_LOCK_ROOT"), os.Getenv("AP_RECORD_LOCK_RUN_ID"))
	result := os.Getenv("AP_RECORD_LOCK_RESULT")
	if err != nil {
		prefix := "error:"
		if apperr.IsUsage(err) {
			prefix = "usage:"
		}
		mustWriteHelperResult(result, prefix+strings.TrimPrefix(err.Error(), apperr.ErrUsage.Error()+": "))
		return
	}
	mustWriteHelperResult(result, "acquired")
	if os.Getenv("AP_RECORD_LOCK_MODE") == "crash" {
		os.Exit(23)
	}
	waitForPath(os.Getenv("AP_RECORD_LOCK_RELEASE"))
	if err := lock.Release(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(24)
	}
}

func recordLockHelperCommand(root, runID, mode, start, result, release string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecordLockProcessHelper$")
	cmd.Env = append(os.Environ(),
		recordLockHelperEnv+"=1",
		"AP_RECORD_LOCK_ROOT="+root,
		"AP_RECORD_LOCK_RUN_ID="+runID,
		"AP_RECORD_LOCK_MODE="+mode,
		"AP_RECORD_LOCK_START="+start,
		"AP_RECORD_LOCK_RESULT="+result,
		"AP_RECORD_LOCK_RELEASE="+release,
	)
	return cmd
}

type recordLockHelperProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func monitorRecordLockHelper(cmd *exec.Cmd) *recordLockHelperProcess {
	helper := &recordLockHelperProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		helper.err = cmd.Wait()
		close(helper.done)
	}()
	return helper
}

func cleanupRecordLockHelper(t *testing.T, helper *recordLockHelperProcess, release string) {
	t.Helper()
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		killAndReapRecordLockHelper(t, helper)
		t.Errorf("signal record lock helper release: %v", err)
		return
	}
	select {
	case <-helper.done:
		if helper.err != nil {
			t.Errorf("record lock helper failed: %v", helper.err)
		}
	case <-time.After(2 * time.Second):
		killAndReapRecordLockHelper(t, helper)
		t.Error("timed out waiting for record lock helper; killed and reaped process")
	}
}

func killAndReapRecordLockHelper(t *testing.T, helper *recordLockHelperProcess) {
	t.Helper()
	if err := helper.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("kill record lock helper: %v", err)
	}
	select {
	case <-helper.done:
	case <-time.After(2 * time.Second):
		t.Error("timed out reaping record lock helper after kill")
	}
}

func waitForRecordLockHelper(t *testing.T, helper *recordLockHelperProcess, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			last = strings.TrimSpace(string(data))
			if completeRecordLockHelperResult(last) {
				return last
			}
		} else {
			lastErr = err
		}
		select {
		case <-helper.done:
			t.Fatalf("record lock helper exited before publishing a complete result: %v (last result %q, read error %v)", helper.err, last, lastErr)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for complete record lock helper result at %s (last result %q, read error %v)", path, last, lastErr)
	return ""
}

func completeRecordLockHelperResult(result string) bool {
	return result == "acquired" ||
		strings.HasPrefix(result, "usage:") && len(result) > len("usage:") ||
		strings.HasPrefix(result, "error:") && len(result) > len("error:")
}

func waitForPath(path string) {
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func mustWriteHelperResult(path, result string) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err == nil {
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if _, err = tmp.Write([]byte(result)); err == nil {
			err = tmp.Close()
		} else {
			_ = tmp.Close()
		}
		if err == nil {
			err = os.Rename(tmpPath, path)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(25)
	}
}
