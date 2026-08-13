package completion_test

import (
	"bytes"
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
