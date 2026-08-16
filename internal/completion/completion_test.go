package completion_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralabarta/agentproof/internal/completion"
)

// commands mirrors the CLI surface wired in internal/app. Keeping the list in
// sync is intentional: a new command must be added to both places, and this
// test fails when a completion script stops mentioning one.
var commands = []string{"init", "record", "verify", "purge", "runs", "status", "doctor", "completion"}

func TestGenerateBash(t *testing.T) {
	out := generate(t, "bash")
	for _, marker := range []string{"_agentproof()", "complete -F _agentproof agentproof"} {
		if !strings.Contains(out, marker) {
			t.Errorf("bash completion missing %q", marker)
		}
	}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("bash completion missing command %q", cmd)
		}
	}
	for _, flag := range []string{"--force", "--objective", "--test-result", "--fail-on", "--older-than"} {
		if !strings.Contains(out, flag) {
			t.Errorf("bash completion missing flag %q", flag)
		}
	}
}

func TestGenerateZsh(t *testing.T) {
	out := generate(t, "zsh")
	if !strings.HasPrefix(out, "#compdef agentproof\n") {
		t.Errorf("zsh completion must start with the #compdef marker, got %q", out[:30])
	}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("zsh completion missing command %q", cmd)
		}
	}
}

func TestGenerateZshRegistersDirectSourceWithCompdef(t *testing.T) {
	out := generate(t, "zsh")
	if !strings.Contains(out, "\ncompdef _agentproof agentproof\n") {
		t.Fatal("zsh completion must register _agentproof with compdef")
	}

	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}
	tempDir := t.TempDir()
	cmd := exec.Command("zsh", "-fc", "autoload -Uz compinit; compinit -D; source /dev/stdin; (( $+_comps[agentproof] ))")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "FPATH=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+tempDir, "ZDOTDIR="+tempDir)
	cmd.Stdin = strings.NewReader(out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("source generated zsh completion: %v: %s", err, output)
	}
	if dumps, err := filepath.Glob(filepath.Join(tempDir, ".zcompdump*")); err != nil {
		t.Fatalf("glob zsh completion dumps: %v", err)
	} else if len(dumps) != 0 {
		t.Errorf("compinit created completion dump files: %v", dumps)
	}
}

func TestGenerateZshDoesNotInvokeCompletionDuringSource(t *testing.T) {
	out := generate(t, "zsh")
	if strings.Contains(out, "\n_agentproof \"$@\"\n") {
		t.Fatal("zsh completion must not invoke _agentproof while being sourced")
	}
}

func TestGeneratePurgeIncludesRunsSelector(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		if out := generate(t, shell); !strings.Contains(out, "--runs") {
			t.Errorf("%s completion missing purge selector --runs", shell)
		}
	}
}

func TestGenerateFish(t *testing.T) {
	out := generate(t, "fish")
	if !strings.Contains(out, "complete -c agentproof") {
		t.Error("fish completion must register completions for agentproof")
	}
	for _, cmd := range commands {
		if !strings.Contains(out, cmd) {
			t.Errorf("fish completion missing command %q", cmd)
		}
	}
}

func TestCommandOptionsReturnsCopy(t *testing.T) {
	options := completion.CommandOptions("purge")
	if len(options) == 0 {
		t.Fatal("purge command options must not be empty")
	}
	options[0] = "--mutated"
	if fresh := completion.CommandOptions("purge"); len(fresh) == 0 || fresh[0] == "--mutated" {
		t.Fatalf("CommandOptions leaked mutable command state: %v", fresh)
	}
}

func TestGenerateUnsupportedShell(t *testing.T) {
	var buf bytes.Buffer
	err := completion.Generate("powershell", &buf)
	if err == nil {
		t.Fatal("unsupported shell must return an error")
	}
	if !strings.Contains(err.Error(), "powershell") {
		t.Errorf("error must name the unsupported shell, got %v", err)
	}
	if buf.Len() != 0 {
		t.Error("no output may be written for an unsupported shell")
	}
}

func generate(t *testing.T, shell string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := completion.Generate(shell, &buf); err != nil {
		t.Fatalf("Generate(%q): %v", shell, err)
	}
	return buf.String()
}
