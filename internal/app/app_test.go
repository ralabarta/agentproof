package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The exit codes are a documented public contract that CI systems branch on:
// 2 is invalid usage or configuration, 3 is an internal or adapter failure.
// Collapsing a missing repository or a missing configuration into 3 tells a
// reviewer that AgentProof broke when the invocation was simply wrong.
func TestUsageErrorsExitTwo(t *testing.T) {
	chdir(t, t.TempDir())
	for _, args := range [][]string{
		{"init"},
		{"verify"},
		{"purge", "--raw"},
	} {
		if code, err := Run(args, "test"); code != 2 {
			t.Errorf("%v outside a Git repository is invalid usage: got %d (%v)", args, code, err)
		}
	}
}

func TestVerifyWithoutInitIsUsageNotInternalFailure(t *testing.T) {
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"verify"}, "test"); code != 2 {
		t.Fatalf("an uninitialized repository is invalid configuration: got %d (%v)", code, err)
	}
}

func TestVerifyWithoutEvidenceSourceIsUsage(t *testing.T) {
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"init"}, "test"); code != 0 {
		t.Fatalf("init should succeed: got %d (%v)", code, err)
	}
	if code, err := Run([]string{"verify"}, "test"); code != 2 {
		t.Fatalf("no recorded session and no --base is invalid usage: got %d (%v)", code, err)
	}
}

func TestReInitWithoutForceIsUsage(t *testing.T) {
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"init"}, "test"); code != 0 {
		t.Fatalf("first init should succeed: got %d (%v)", code, err)
	}
	if code, err := Run([]string{"init"}, "test"); code != 2 {
		t.Fatalf("re-init without --force is invalid usage: got %d (%v)", code, err)
	}
}

// A recorded agent can exit with any code it likes. Forwarding it would let a
// child's 2 be read as "invalid AgentProof usage" and a child's 3 as an
// AgentProof internal failure.
func TestRecordDoesNotForwardChildExitCode(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"init"}, "test"); code != 0 {
		t.Fatalf("init should succeed: got %d (%v)", code, err)
	}
	code, err := Run([]string{"record", "--objective", "x", "--", "sh", "-c", "exit 42"}, "test")
	if code == 42 {
		t.Fatal("the child exit code must not become AgentProof's exit code")
	}
	if code != 1 {
		t.Fatalf("a failed recorded command is a failed run: got %d (%v)", code, err)
	}
}

func TestRecordWithoutCommandIsUsage(t *testing.T) {
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"record", "--objective", "x"}, "test"); code != 2 {
		t.Fatalf("record without a command after -- is invalid usage: got %d (%v)", code, err)
	}
}

func TestCompletionCommand(t *testing.T) {
	// Completions do not depend on a Git repository, so a plain temp dir is fine.
	chdir(t, t.TempDir())
	if code, err := Run([]string{"completion"}, "test"); code != 0 {
		t.Fatalf("completion with default shell should succeed: got %d (%v)", code, err)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		if code, err := Run([]string{"completion", shell}, "test"); code != 0 {
			t.Fatalf("completion %s should succeed: got %d (%v)", shell, code, err)
		}
	}
	if code, err := Run([]string{"completion", "powershell"}, "test"); code != 2 {
		t.Fatalf("an unsupported shell is invalid usage: got %d (%v)", code, err)
	}
	if code, err := Run([]string{"completion", "bash", "zsh"}, "test"); code != 2 {
		t.Fatalf("more than one shell is invalid usage: got %d (%v)", code, err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	// t.TempDir can sit behind a symlink that git resolves, so resolve it here
	// too or the recorded root will not match the working directory.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return root
}
