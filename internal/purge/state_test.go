package purge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunsRejectsSymlinkedStateFile(t *testing.T) {
	root := t.TempDir()
	run := writeRun(t, root, "symlinked-state", "")
	external, sentinel := writeExternalSentinel(t)
	state := filepath.Join(external, "state.json")
	if err := os.WriteFile(state, []byte(`{"status":"abandoned"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(run, "state.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(state, filepath.Join(run, "state.json")); err != nil {
		t.Fatal(err)
	}

	result := Runs(root, Options{Confirm: true})
	if result.Selected != 0 || result.Deleted != 0 || result.Failed != 1 {
		t.Fatalf("symlinked state should fail closed: %#v", result)
	}
	assertPathExists(t, run)
	assertPathExists(t, sentinel)
}

func TestRunsRejectsOversizedStateFile(t *testing.T) {
	root := t.TempDir()
	run := writeRun(t, root, "oversized-state", "")
	_, sentinel := writeExternalSentinel(t)
	data := `{"status":"abandoned"}` + strings.Repeat(" ", 64<<10)
	if err := os.WriteFile(filepath.Join(run, "state.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Runs(root, Options{Confirm: true})
	if result.Selected != 0 || result.Deleted != 0 || result.Failed != 1 {
		t.Fatalf("oversized state should fail closed: %#v", result)
	}
	assertPathExists(t, run)
	assertPathExists(t, sentinel)
}
