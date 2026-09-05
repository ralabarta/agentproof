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

func TestGenerateBashSuggestsSubcommandOptionsAfterArguments(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}

	script := generate(t, "bash")
	cmd := exec.Command("bash", "-c", script+`
COMP_WORDS=(agentproof verify report.json --)
COMP_CWORD=3
_agentproof
printf '%s\n' "${COMPREPLY[@]}"
`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated bash completion: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "--base\n") {
		t.Fatalf("bash completion must keep suggesting verify options after arguments, got %q", output)
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

func TestGenerateZshEqualsFiniteFlagSyntax(t *testing.T) {
	out := generate(t, "zsh")
	branches := []string{
		"--agent=*) compset -P '1 *='; compadd -P '--agent=' -- codex claude claude-code; return 0 ;;",
		"--fail-on=*) compset -P '1 *='; compadd -P '--fail-on=' -- critical high medium low none; return 0 ;;",
	}
	for _, branch := range branches {
		if count := strings.Count(out, branch); count != 1 {
			t.Errorf("zsh completion contains equals branch %q %d times, want exactly once", branch, count)
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

func TestGenerateFishScopesFiniteFlagValues(t *testing.T) {
	out := generate(t, "fish")
	var declarations []string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.Contains(line, " -r -f ") {
			declarations = append(declarations, line)
		}
	}

	want := []string{
		"complete -c agentproof -n '__fish_seen_subcommand_from record' -l agent -r -f -a 'codex claude claude-code'",
		"complete -c agentproof -n '__fish_seen_subcommand_from verify' -l fail-on -r -f -a 'critical high medium low none'",
	}
	if strings.Join(declarations, "\n") != strings.Join(want, "\n") {
		t.Fatalf("fish finite-value declarations = %q, want exactly %q", declarations, want)
	}
}

type completionScenario struct {
	name   string
	want   []string
	reject []string
}

var finiteValueScenarios = []completionScenario{
	{name: "record_separate", want: []string{"codex", "claude", "claude-code"}},
	{name: "record_equals_prefix", want: []string{"--agent=claude", "--agent=claude-code"}},
	{name: "verify_separate", want: []string{"critical", "high", "medium", "low", "none"}},
	{name: "verify_equals_prefix", want: []string{"--fail-on=medium"}},
	{name: "record_rejects_fail_on", reject: []string{"critical", "high", "medium", "low", "none"}},
	{name: "verify_rejects_agent", reject: []string{"codex", "claude", "claude-code"}},
}

func TestGenerateBashCompletesFiniteFlagValues(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}

	script := generate(t, "bash") + `
run_case() {
    local name="$1" cword="$2"
    shift 2
    COMP_WORDS=("$@")
    COMP_CWORD="$cword"
    COMPREPLY=()
    _agentproof
    printf '__CASE__%s\n' "$name"
    printf '%s\n' "${COMPREPLY[@]}"
    printf '__END__\n'
}
run_case record_separate 3 agentproof record --agent ""
run_case record_equals_prefix 2 agentproof record --agent=cl
run_case verify_separate 3 agentproof verify --fail-on ""
run_case verify_equals_prefix 2 agentproof verify --fail-on=m
run_case record_rejects_fail_on 3 agentproof record --fail-on ""
run_case verify_rejects_agent 3 agentproof verify --agent ""
`
	assertCompletionScenarios(t, "bash", exec.Command("bash", "-c", script), finiteValueScenarios)
}

func TestGenerateZshCompletesFiniteFlagValues(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	script := `
compdef() { : }
compset() {
  if [[ "$1" == "-P" && "$2" == "1 *=" ]]; then
    match="${words[CURRENT]#*=}"
  fi
}
compadd() {
  local prefix=""
  if [[ "$1" == "-P" ]]; then
    prefix="$2"
    shift 2
  fi
  [[ "$1" == "--" ]] && shift
  local candidate
  for candidate in "$@"; do
    if [[ -z "$match" || "$candidate" == "$match"* ]]; then
      candidates+=("${prefix}${candidate}")
    fi
  done
}
` + generate(t, "zsh") + `
run_case() {
  local name="$1" current="$2"
  shift 2
  words=("$@")
  CURRENT="$current"
  match="${words[CURRENT]}"
  candidates=()
  _agentproof
  print -r -- "__CASE__${name}"
  print -rl -- "${candidates[@]}"
  print -r -- '__END__'
}
run_case record_separate 4 agentproof record --agent ""
run_case record_equals_prefix 3 agentproof record --agent=cl
run_case verify_separate 4 agentproof verify --fail-on ""
run_case verify_equals_prefix 3 agentproof verify --fail-on=m
run_case record_rejects_fail_on 4 agentproof record --fail-on ""
run_case verify_rejects_agent 4 agentproof verify --agent ""
`
	assertCompletionScenarios(t, "zsh", exec.Command("zsh", "-fc", script), finiteValueScenarios)
}

func TestGenerateFishCompletesFiniteFlagValues(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish is not installed")
	}

	tempDir := t.TempDir()
	for _, name := range []string{"filesystem-decoy", "cl-filesystem-decoy", "m-filesystem-decoy"} {
		if err := os.WriteFile(filepath.Join(tempDir, name), nil, 0o600); err != nil {
			t.Fatalf("create filesystem completion decoy %q: %v", name, err)
		}
	}

	script := generate(t, "fish") + `
function run_case
    printf '__CASE__%s\n' "$argv[1]"
    complete --do-complete "$argv[2]" | string replace -r '\t.*$' ''
    printf '__END__\n'
end
run_case record_separate 'agentproof record --agent '
run_case record_equals_prefix 'agentproof record --agent=cl'
run_case verify_separate 'agentproof verify --fail-on '
run_case verify_equals_prefix 'agentproof verify --fail-on=m'
`
	fishScenarios := append([]completionScenario(nil), finiteValueScenarios[:4]...)
	fishScenarios[0].want = []string{"claude", "claude-code", "codex"}
	fishScenarios[2].want = []string{"critical", "high", "low", "medium", "none"}
	cmd := exec.Command("fish", "-c", script)
	cmd.Dir = tempDir
	assertCompletionScenarios(t, "fish", cmd, fishScenarios)
}

func assertCompletionScenarios(t *testing.T, shell string, cmd *exec.Cmd, scenarios []completionScenario) {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated %s completion: %v: %s", shell, err, output)
	}

	results := parseCompletionScenarios(t, shell, string(output))
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			got, ok := results[scenario.name]
			if !ok {
				t.Fatalf("generated %s completion did not report scenario; full output: %q", shell, output)
			}
			if scenario.want != nil && strings.Join(got, "\n") != strings.Join(scenario.want, "\n") {
				t.Fatalf("%s candidates = %q, want %q", shell, got, scenario.want)
			}
			for _, rejected := range scenario.reject {
				for _, candidate := range got {
					if candidate == rejected {
						t.Errorf("%s candidates unexpectedly include command-scoped value %q: %q", shell, rejected, got)
					}
				}
			}
		})
	}
}

func parseCompletionScenarios(t *testing.T, shell, output string) map[string][]string {
	t.Helper()
	results := make(map[string][]string)
	var current string
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "__CASE__"):
			current = strings.TrimPrefix(line, "__CASE__")
			results[current] = nil
		case line == "__END__":
			current = ""
		case current == "":
			t.Fatalf("unexpected generated %s completion output line %q", shell, line)
		default:
			results[current] = append(results[current], line)
		}
	}
	return results
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
