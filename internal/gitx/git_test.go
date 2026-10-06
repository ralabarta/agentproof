package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/apperr"
)

// TestCompareBaseRejectsUnknownRef guards the exit-code contract: a typo'd
// --base is a fixable invocation, so CompareBase must classify it as usage
// (exit 2 via verify) instead of letting raw git stderr surface as an
// internal failure. A valid ref must keep behaving exactly as before.
func TestCompareBaseRejectsUnknownRef(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@agentproof.dev")
	git(t, root, "config", "user.name", "AgentProof Test")
	writeFile(t, filepath.Join(root, "README.md"), "base\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "base")

	if _, _, err := CompareBase(root, "nonexistent-ref"); !errors.Is(err, apperr.ErrUsage) {
		t.Fatalf("CompareBase(unknown) error = %v, want ErrUsage", err)
	}
	if _, _, err := CompareBase(root, "HEAD"); err != nil {
		t.Fatalf("valid base must keep working: %v", err)
	}
}

func TestCollectIncludesUntrackedText(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@agentproof.dev")
	git(t, root, "config", "user.name", "AgentProof Test")
	writeFile(t, filepath.Join(root, "README.md"), "base\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "base")
	start, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "new.go"), "package example\n\nfunc Added() {}\n")
	end, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, patch, err := Collect(root, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.Changes) != 1 || repo.Changes[0].Status != "untracked" || repo.Changes[0].Added != 3 {
		t.Fatalf("unexpected changes: %#v", repo.Changes)
	}
	if len(repo.CommittedChanges) != 0 || len(repo.WorkingChanges) != 1 {
		t.Fatalf("committed and working changes were not separated: %#v", repo)
	}
	if !strings.Contains(patch, "func Added") {
		t.Fatalf("untracked file was not included in patch: %s", patch)
	}
}

func TestCollectIncludesUntrackedUTF8Path(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@agentproof.dev")
	git(t, root, "config", "user.name", "AgentProof Test")
	writeFile(t, filepath.Join(root, "README.md"), "base\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "base")
	start, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "café.go"), "package example\n\nfunc Added() {}\n")
	end, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, patch, err := Collect(root, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.Changes) != 1 || repo.Changes[0].Path != "café.go" || repo.Changes[0].Status != "untracked" || repo.Changes[0].Added != 3 {
		t.Fatalf("unexpected changes: %#v", repo.Changes)
	}
	if len(repo.UncapturedPaths) != 0 {
		t.Fatalf("UTF-8 text path was classified as uncaptured: %#v", repo.UncapturedPaths)
	}
	if !strings.Contains(patch, "func Added") {
		t.Fatalf("UTF-8 path was not included in patch: %s", patch)
	}
}

func TestCollectTracksUTF8PathForTrackedModification(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@agentproof.dev")
	git(t, root, "config", "user.name", "AgentProof Test")
	path := filepath.Join(root, "café.go")
	writeFile(t, path, "package example\n\nfunc Base() {}\n")
	git(t, root, "add", "café.go")
	git(t, root, "commit", "-m", "base")
	start, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "package example\n\nfunc Base() {}\n\nfunc Added() {}\n")
	end, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, _, err := Collect(root, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.WorkingChanges) != 1 || repo.WorkingChanges[0].Path != "café.go" {
		t.Fatalf("tracked UTF-8 working path was quoted or missing: %#v", repo.WorkingChanges)
	}
	if len(repo.Changes) != 1 || repo.Changes[0].Path != "café.go" {
		t.Fatalf("tracked UTF-8 combined path was quoted or missing: %#v", repo.Changes)
	}

	git(t, root, "add", "café.go")
	git(t, root, "commit", "-m", "track utf-8 change")
	committedStart := start
	committedEnd, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	committed, _, err := Collect(root, committedStart, committedEnd)
	if err != nil {
		t.Fatal(err)
	}
	if len(committed.CommittedChanges) != 1 || committed.CommittedChanges[0].Path != "café.go" {
		t.Fatalf("tracked UTF-8 committed path was quoted or missing: %#v", committed.CommittedChanges)
	}
}

func TestCollectMarksUntrackedBinaryAsUncaptured(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@agentproof.dev")
	git(t, root, "config", "user.name", "AgentProof Test")
	writeFile(t, filepath.Join(root, "README.md"), "base\n")
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "base")
	start, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image.bin"), []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	end, err := TakeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	repo, _, err := Collect(root, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.UncapturedPaths) != 1 || repo.UncapturedPaths[0] != "image.bin" || repo.AssociationStatus != "unknown-uncaptured-worktree" {
		t.Fatalf("binary evidence gap was not represented: %#v", repo)
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
