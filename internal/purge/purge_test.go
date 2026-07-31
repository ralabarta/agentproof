package purge

import (
	"os"
	"path/filepath"
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
