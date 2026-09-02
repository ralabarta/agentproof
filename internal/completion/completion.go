// Package completion renders shell completion scripts for the agentproof CLI.
// The command and option table below is the single source of truth: the same
// table drives bash, zsh, and fish output, so adding a command in
// internal/app keeps every completion script in sync.
package completion

import (
	"fmt"
	"io"
	"text/template"
)

// commandSpec describes one CLI command and the option words a shell should
// suggest after it. Args are plain tokens — flags or positional values such as
// the completion shell names — and Desc is only used by templates that can
// display it (fish).
type commandSpec struct {
	Name string
	Desc string
	Args []string
}

// commands must stay aligned with the switch in internal/app.Run. The package
// test fails when a completion script stops mentioning a command, so a new CLI
// command without an entry here is caught, not silently omitted.
var commands = []commandSpec{
	{Name: "init", Desc: "Create a local-first AgentProof configuration", Args: []string{"--force"}},
	{Name: "record", Desc: "Record an agent command and its Git change window", Args: []string{"--objective", "--agent", "--model", "--retain-raw"}},
	{Name: "verify", Desc: "Ingest evidence and generate deterministic integrity reports", Args: []string{"--base", "--test-result", "--require-tests", "--fail-on"}},
	{Name: "purge", Desc: "Preview or delete opted-in raw command logs", Args: []string{"--raw", "--runs", "--older-than", "--confirm"}},
	{Name: "runs", Desc: "List recorded runs"},
	{Name: "status", Desc: "Show AgentProof state"},
	{Name: "doctor", Desc: "Run diagnostic checks"},
	{Name: "completion", Desc: "Generate a shell completion script", Args: []string{"bash", "zsh", "fish"}},
}

const bashTmpl = `# bash completion for agentproof
_agentproof() {
    local cur command
    cur="${COMP_WORDS[COMP_CWORD]}"
    command="${COMP_WORDS[1]}"
    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "{{range .}}{{.Name}} {{end}}" -- "${cur}") )
        return 0
    fi
    case "${command}" in
{{range .}}        {{.Name}})
            COMPREPLY=( $(compgen -W "{{range .Args}}{{.}} {{end}}" -- "${cur}") )
            ;;
{{end}}        *)
            COMPREPLY=( $(compgen -f -- "${cur}") )
            ;;
    esac
    return 0
}
complete -F _agentproof agentproof
`

const zshTmpl = `#compdef agentproof
_agentproof() {
    if (( CURRENT == 2 )); then
        compadd -- {{range .}}{{.Name}} {{end}}
        return 0
    fi
    case "${words[2]}" in
{{range .}}        {{.Name}})
            compadd -- {{range .Args}}{{.}} {{end}}
            ;;
{{end}}    esac
    return 0
}
compdef _agentproof agentproof
`

const fishTmpl = `# fish completion for agentproof
{{range .}}complete -c agentproof -n '__fish_use_subcommand' -a '{{.Name}}' -d '{{.Desc}}'
{{end}}{{range .}}{{if .Args}}complete -c agentproof -n '__fish_seen_subcommand_from {{.Name}}' -a '{{range .Args}}{{.}} {{end}}'
{{end}}{{end}}`

// CommandOptions returns a copy of the completion options for name.
func CommandOptions(name string) []string {
	for _, command := range commands {
		if command.Name == name {
			return append([]string(nil), command.Args...)
		}
	}
	return nil
}

// Generate writes a completion script for the requested shell. An unsupported
// shell is an error and writes nothing.
func Generate(shell string, w io.Writer) error {
	var source string
	switch shell {
	case "bash":
		source = bashTmpl
	case "zsh":
		source = zshTmpl
	case "fish":
		source = fishTmpl
	default:
		return fmt.Errorf("unsupported shell %q; use bash, zsh, or fish", shell)
	}
	tmpl, err := template.New("completion").Parse(source)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, commands)
}
