package purge

import (
	"os"
	"path/filepath"
	"strconv"
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

// While a record is in progress its lock is live and a recording-state run is
// indistinguishable from it, so purge conservatively leaves every recording
// run alone. Abandoned runs are dead regardless of the live record.
func TestRunsSparesRecordingWhileRecordIsLive(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "stuck", "recording")
	abandoned := writeRun(t, root, "abandoned", "abandoned")
	if err := os.WriteFile(filepath.Join(root, ".agentproof", ".record.lock"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Runs(root, Options{OlderThan: 0, Confirm: true})
	if result.Selected != 1 || result.Deleted != 1 || result.Failed != 0 {
		t.Fatalf("only the abandoned run should be selected while a record is live: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, ".agentproof", "runs", "stuck")); err != nil {
		t.Fatalf("recording run must survive while a record is live: %v", err)
	}
	if _, err := os.Stat(abandoned); !os.IsNotExist(err) {
		t.Fatal("abandoned run directory still exists")
	}
}
