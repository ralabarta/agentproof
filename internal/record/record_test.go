package record

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
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

// A clean exit must leave the run lifecycle state as complete, with both
// timestamps populated.
func TestStateFileCompleteOnCleanExit(t *testing.T) {
	root := newRepo(t)
	run, err := Run(root, Options{Objective: "clean", Command: []string{"sh", "-c", "true"}})
	if err != nil {
		t.Fatal(err)
	}
	state := readState(t, filepath.Join(root, config.DirName, "runs", run.RunID, "state.json"))
	if state.Status != "complete" {
		t.Fatalf("expected complete, got %q", state.Status)
	}
	if state.StartedAt.IsZero() || state.CompletedAt == nil {
		t.Fatalf("expected startedAt and completedAt, got %#v", state)
	}
}

// The run lifecycle JSON is consumed by internal/status, so its keys are a
// contract. Pin them here so a rename cannot silently break abandoned-run
// detection.
func TestRunStateJSONContract(t *testing.T) {
	b, err := json.Marshal(runState{Status: "abandoned", StartedAt: time.Unix(0, 0).UTC(), Signal: "SIGINT"})
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["status"] != "abandoned" || raw["signal"] != "SIGINT" {
		t.Fatalf("state keys drifted from the contract read by status.Read: %v", raw)
	}
	if _, ok := raw["startedAt"]; !ok {
		t.Fatal("startedAt must be present")
	}
}

// A live lock must fail closed: two records in one repository would write
// overlapping windows and poison the Git association evidence.
func TestRecordLockRejectsParallelRecord(t *testing.T) {
	root := newRepo(t)
	lockPath := filepath.Join(root, config.DirName, ".record.lock")
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(root, Options{Objective: "x", Command: []string{"sh", "-c", "true"}})
	if err == nil {
		t.Fatal("expected a live lock to block a second record")
	}
	if !apperr.IsUsage(err) {
		t.Fatalf("lock contention should be usage-classified, got %v", err)
	}
}

// A lock left behind by a dead process is stale and must not block; after the
// run the lock file is gone again.
func TestRecordLockIgnoresStaleLock(t *testing.T) {
	root := newRepo(t)
	dead := exec.Command("sh", "-c", "exit 0")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(root, config.DirName, ".record.lock")
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(dead.ProcessState.Pid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, Options{Objective: "x", Command: []string{"sh", "-c", "true"}}); err != nil {
		t.Fatalf("a stale lock must not block a record: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock must be released after the run, got %v", err)
	}
}

// An interrupt while recording must mark the run abandoned. The recording runs
// in a re-exec'd child so the signal reaches the real signal handler; the
// parent first waits for the recording state so the handler is armed.
func TestStateFileAbandonedOnInterrupt(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	if runtime.GOOS == "windows" {
		t.Skip("signal semantics differ on windows")
	}
	root := newRepo(t)
	cmd := exec.Command(os.Args[0], "-test.run=TestRecordHelper", "--")
	cmd.Env = append(os.Environ(), "AP_TEST_CHILD=1", "AP_TEST_ROOT="+root)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if !waitForStatus(t, root, "recording") {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("the child never reached the recording state")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if !waitForStatus(t, root, "abandoned") {
		t.Fatal("state.json never became abandoned after SIGINT")
	}
}

// TestRecordHelper is not a test: it is the re-exec'd child that runs a real
// record while TestStateFileAbandonedOnInterrupt signals it.
func TestRecordHelper(t *testing.T) {
	if os.Getenv("AP_TEST_CHILD") != "1" {
		return
	}
	_, _ = Run(os.Getenv("AP_TEST_ROOT"), Options{Objective: "interrupt me", Command: []string{"sh", "-c", "sleep 2"}})
}

func readState(t *testing.T, path string) runState {
	t.Helper()
	var state runState
	if err := json.Unmarshal([]byte(readFile(t, path)), &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// waitForStatus polls every run directory until one state.json carries the
// requested status or the deadline passes.
func waitForStatus(t *testing.T, root, want string) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(root, config.DirName, "runs", "*", "state.json"))
		for _, path := range matches {
			b, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var state runState
			if json.Unmarshal(b, &state) == nil && state.Status == want {
				return true
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}
