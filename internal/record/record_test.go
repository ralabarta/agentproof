package record

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/config"
)

// Raw command output is the one evidence file AgentProof copies from a process
// it does not control. The same bytes are redacted in changes.patch, so a
// retained log that keeps them verbatim is the widest secret leak in a bundle.
func TestRecordRedactsRetainedCommandOutput(t *testing.T) {
	root := newRepo(t)
	run, err := Run(root, Options{
		Objective: "print a secret",
		Command:   []string{"sh", "-c", "echo deploying with AKIAABCDEFGHIJKLMNOP to prod"},
		RetainRaw: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	log := readFile(t, filepath.Join(root, config.DirName, "runs", run.RunID, "command.log"))
	if strings.Contains(log, "AKIAABCDEFGHIJKLMNOP") {
		t.Fatalf("secret survived in the retained log: %q", log)
	}
	if !strings.Contains(log, "[REDACTED:AP-SECRET-001]") {
		t.Fatalf("expected a redaction marker in the retained log: %q", log)
	}
	if !strings.Contains(log, "deploying with") {
		t.Fatalf("surrounding output was lost: %q", log)
	}
}

// Retention is opt-in, so a default run must leave nothing behind to purge.
func TestRecordWritesNoLogWithoutRetention(t *testing.T) {
	root := newRepo(t)
	run, err := Run(root, Options{Objective: "quiet", Command: []string{"sh", "-c", "echo hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, config.DirName, "runs", run.RunID, "command.log")); !os.IsNotExist(err) {
		t.Fatalf("a raw log was written without retention: %v", err)
	}
}

// A failing command still produced evidence, so its exit code is recorded and
// surfaced rather than discarded with the error.
func TestRecordReportsTheCommandExitCode(t *testing.T) {
	root := newRepo(t)
	run, err := Run(root, Options{Objective: "fail", Command: []string{"sh", "-c", "exit 3"}})
	if err == nil {
		t.Fatal("expected a failing command to be reported")
	}
	if run.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", run.ExitCode)
	}
}

func TestRecordRejectsAnEmptyCommand(t *testing.T) {
	if _, err := Run(newRepo(t), Options{Objective: "none"}); err == nil {
		t.Fatal("expected an empty command to be rejected")
	}
}

func TestRecordRequiresInitialization(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	if _, err := Run(root, Options{Objective: "none", Command: []string{"sh", "-c", "true"}}); err == nil {
		t.Fatal("expected an uninitialized repository to be rejected")
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitInit(t, root)
	if err := config.Init(root, false); err != nil {
		t.Fatal(err)
	}
	return root
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
