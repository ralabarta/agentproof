package safefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.json")
	if err := Write(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "second" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestWriteRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Write(link, []byte("unsafe"), 0o600); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
