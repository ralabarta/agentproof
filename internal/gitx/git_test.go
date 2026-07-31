package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
