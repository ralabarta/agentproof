package app

import (
	"flag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/completion"
)

func TestRootHelpListsPublicCommands(t *testing.T) {
	output := captureStdout(t, func() {
		if code, err := Run([]string{"--help"}, "test"); code != 0 || err != nil {
			t.Fatalf("root help should succeed: got %d (%v)", code, err)
		}
	})
	for _, command := range []string{"init", "record", "verify", "runs", "status", "doctor", "purge", "completion"} {
		if !strings.Contains(output, "\n  "+command+" ") {
			t.Errorf("root help does not list %q:\n%s", command, output)
		}
	}
	if !strings.Contains(output, "agentproof purge --runs --older-than 168h [--confirm]") {
		t.Errorf("root help does not include purge --runs guidance:\n%s", output)
	}
}

func TestPurgeHelpPrintsOptionsAndSucceeds(t *testing.T) {
	output := captureStdout(t, func() {
		if code, err := Run([]string{"purge", "--help"}, "test"); code != 0 || err != nil {
			t.Fatalf("purge help should succeed: got %d (%v)", code, err)
		}
	})
	for _, option := range []string{"-raw", "-runs", "-older-than", "-confirm"} {
		if !strings.Contains(output, option) {
			t.Errorf("purge help does not list %q:\n%s", option, output)
		}
	}
}

func TestPurgeUnknownFlagDoesNotWriteToStdout(t *testing.T) {
	output := captureStdout(t, func() {
		code, err := Run([]string{"purge", "--unknown"}, "test")
		if code != 2 {
			t.Fatalf("unknown purge flag should be invalid usage: got %d (%v)", code, err)
		}
		if err == nil {
			t.Fatal("unknown purge flag should return an error")
		}
	})
	if output != "" {
		t.Fatalf("unknown purge flag wrote to stdout: %q", output)
	}
}

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
		{"purge", "--runs"},
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

// Purge requires a selector: without --raw or --runs there is nothing to
// preview or delete, and the invocation is invalid usage.
func TestPurgeRequiresASelector(t *testing.T) {
	chdir(t, gitRepo(t))
	if code, err := Run([]string{"purge"}, "test"); code != 2 {
		t.Fatalf("purge without a selector is invalid usage: got %d (%v)", code, err)
	}
}

func TestCompletionPurgeOptionsMatchPurgeParser(t *testing.T) {
	fs, _ := newPurgeFlagSet()
	var parserOptions []string
	fs.VisitAll(func(f *flag.Flag) {
		parserOptions = append(parserOptions, "--"+f.Name)
	})
	completionOptions := completion.CommandOptions("purge")
	sort.Strings(parserOptions)
	sort.Strings(completionOptions)
	if !reflect.DeepEqual(completionOptions, parserOptions) {
		t.Fatalf("purge completion options = %v, parser options = %v", completionOptions, parserOptions)
	}
}

func TestPurgeZeroAgeSelectsRecentRun(t *testing.T) {
	root := gitRepo(t)
	chdir(t, root)
	if code, err := Run([]string{"init"}, "test"); code != 0 {
		t.Fatalf("init should succeed: got %d (%v)", code, err)
	}
	runDir := filepath.Join(root, ".agentproof", "runs", "recent")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(`{"status":"abandoned"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if code, err := Run([]string{"purge", "--runs", "--older-than", "0"}, "test"); code != 0 {
		t.Fatalf("zero-age purge preview should succeed: got %d (%v)", code, err)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("preview must preserve selected run: %v", err)
	}
	if code, err := Run([]string{"purge", "--runs", "--older-than", "0", "--confirm"}, "test"); code != 0 {
		t.Fatalf("zero-age purge confirmation should succeed: got %d (%v)", code, err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("zero-age purge did not select and delete recent run: %v", err)
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

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previous })

	run()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
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
