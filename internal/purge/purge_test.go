package purge

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRawPreviewsBeforeDeletion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agentproof", "runs", "one", "command.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("raw"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := Raw(root, Options{OlderThan: 0})
	if preview.Selected != 1 || preview.Deleted != 0 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	deleted := Raw(root, Options{OlderThan: 0, Confirm: true})
	if deleted.Selected != 1 || deleted.Deleted != 1 || deleted.Failed != 0 {
		t.Fatalf("unexpected deletion: %#v", deleted)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("raw file still exists: %v", err)
	}
}

func TestRawHonorsAge(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agentproof", "runs", "one", "command.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("raw"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Raw(root, Options{OlderThan: 24 * time.Hour})
	if result.Selected != 0 {
		t.Fatalf("new raw file should not be selected: %#v", result)
	}
}

func writeRun(t *testing.T, root, name, status string) string {
	t.Helper()
	dir := filepath.Join(root, ".agentproof", "runs", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	state := "{}"
	if status != "" {
		state = `{"status":"` + status + `"}`
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunsPreviewsBeforeDeletion(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "abandoned", "abandoned")

	preview := Runs(root, Options{OlderThan: 0})
	if preview.Selected != 1 || preview.Deleted != 0 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	deleted := Runs(root, Options{OlderThan: 0, Confirm: true})
	if deleted.Selected != 1 || deleted.Deleted != 1 || deleted.Failed != 0 {
		t.Fatalf("unexpected deletion: %#v", deleted)
	}
	if _, err := os.Stat(filepath.Join(root, ".agentproof", "runs", "abandoned")); !os.IsNotExist(err) {
		t.Fatalf("run directory still exists: %v", err)
	}
}

func TestRunsHonorsAge(t *testing.T) {
	root := t.TempDir()
	old := writeRun(t, root, "old", "abandoned")
	fresh := writeRun(t, root, "fresh", "abandoned")
	now := time.Now()
	if err := os.Chtimes(old, now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	result := Runs(root, Options{OlderThan: 48 * time.Hour, Confirm: true})
	if result.Selected != 1 || result.Deleted != 1 || result.Failed != 0 {
		t.Fatalf("only the old run should be selected: %#v", result)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old run directory still exists: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh run directory was deleted: %v", err)
	}
}

// Stuck runs (recording without a live lock) and abandoned runs are dead and
// selectable; completed evidence and runs without a lifecycle state file must
// survive.
func TestRunsSelectsStuckAndAbandonedButNotComplete(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "stuck", "recording")
	writeRun(t, root, "abandoned", "abandoned")
	complete := writeRun(t, root, "complete", "complete")
	unknown := writeRun(t, root, "unknown", "")

	result := Runs(root, Options{OlderThan: 0, Confirm: true})
	if result.Selected != 2 || result.Deleted != 2 || result.Failed != 0 {
		t.Fatalf("stuck and abandoned runs should be selected: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".agentproof", "runs", "stuck")); !os.IsNotExist(err) {
		t.Fatal("stuck run directory still exists")
	}
	if _, err := os.Stat(filepath.Join(root, ".agentproof", "runs", "abandoned")); !os.IsNotExist(err) {
		t.Fatal("abandoned run directory still exists")
	}
	for _, name := range []string{complete, unknown} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("run %s must survive: %v", name, err)
		}
	}
}

func TestRunsSelectsOldRecordingRunWhileDifferentRunIsLive(t *testing.T) {
	root := t.TempDir()
	old := writeRun(t, root, "old-recording", "recording")
	startRecordLockHelper(t, root, "active-recording")

	result := Runs(root, Options{OlderThan: 0, Confirm: true})
	if result.Selected != 1 || result.Deleted != 1 || result.Failed != 0 {
		t.Fatalf("old recording run should be selected despite a different live run: %#v", result)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old recording run still exists: %v", err)
	}
}

func TestRunsSparesRecordingRunWithMatchingLiveLock(t *testing.T) {
	root := t.TempDir()
	active := writeRun(t, root, "active-recording", "recording")
	startRecordLockHelper(t, root, "active-recording")

	result := Runs(root, Options{OlderThan: 0, Confirm: true})
	if result.Selected != 0 || result.Deleted != 0 || result.Failed != 0 {
		t.Fatalf("matching live recording run should not be selected: %#v", result)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("matching live recording run must survive: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".agentproof", ".record.lock"), []byte("malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result = Runs(root, Options{OlderThan: 0, Confirm: true})
	if result.Selected != 0 || result.Deleted != 0 || result.Failed != 0 {
		t.Fatalf("recording run must fail closed while active lock metadata is malformed: %#v", result)
	}
}

func startRecordLockHelper(t *testing.T, root, runID string) {
	t.Helper()
	control := t.TempDir()
	result := filepath.Join(control, "result")
	release := filepath.Join(control, "release")
	cmd := exec.Command("go", "test", "./internal/record", "-run=^TestRecordLockProcessHelper$", "-count=1")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(),
		"AP_RECORD_LOCK_HELPER=1",
		"AP_RECORD_LOCK_ROOT="+root,
		"AP_RECORD_LOCK_RUN_ID="+runID,
		"AP_RECORD_LOCK_MODE=hold",
		"AP_RECORD_LOCK_RESULT="+result,
		"AP_RECORD_LOCK_RELEASE="+release,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	helper := monitorRecordLockHelper(cmd)
	t.Cleanup(func() {
		cleanupRecordLockHelper(t, helper, release)
	})
	if got := waitForRecordLockHelper(t, helper, result); got != "acquired" {
		t.Fatalf("record lock helper result = %q, want acquired", got)
	}
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
			if last == "acquired" {
				return last
			}
		} else {
			lastErr = err
		}
		select {
		case <-helper.done:
			t.Fatalf("record lock helper exited before publishing acquired: %v (last result %q, read error %v)", helper.err, last, lastErr)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for record lock helper to publish acquired at %s (last result %q, read error %v)", path, last, lastErr)
	return ""
}
