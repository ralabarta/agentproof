package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/gitx"
	"github.com/ralabarta/agentproof/internal/purge"
	"github.com/ralabarta/agentproof/internal/record"
	"github.com/ralabarta/agentproof/internal/status"
	"github.com/ralabarta/agentproof/internal/verify"
)

func Run(args []string, version string) (int, error) {
	if len(args) == 0 {
		printHelp(os.Stdout)
		return 0, nil
	}
	switch args[0] {
	case "help", "-h", "--help":
		printHelp(os.Stdout)
		return 0, nil
	case "version", "--version":
		fmt.Fprintln(os.Stdout, version)
		return 0, nil
	case "init":
		return initCommand(args[1:])
	case "record":
		return recordCommand(args[1:])
	case "verify":
		return verifyCommand(args[1:])
	case "purge":
		return purgeCommand(args[1:])
	case "runs":
		return runsCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	default:
		return 2, fmt.Errorf("unknown command %q", args[0])
	}
}

func statusCommand(_ []string) (int, error) {
	cwd, _ := os.Getwd()
	s, err := status.Read(cwd)
	if err != nil {
		return 3, err
	}
	if !s.Initialized {
		fmt.Fprintln(os.Stdout, "Not initialized — run `agentproof init` first")
		return 0, nil
	}
	fmt.Fprintf(os.Stdout, "Initialized:   true\n")
	fmt.Fprintf(os.Stdout, "Runs:          %d\n", s.RunCount)
	fmt.Fprintf(os.Stdout, "Abandoned:     %d\n", s.AbandonedRuns)
	if s.LastStatus != "" {
		fmt.Fprintf(os.Stdout, "Last status:   %s\n", s.LastStatus)
		fmt.Fprintf(os.Stdout, "Last bundle:   %s\n", s.LastBundleID)
		fmt.Fprintf(os.Stdout, "Last verified: %s\n", s.LastVerifiedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	}
	return 0, nil
}


func purgeCommand(args []string) (int, error) {
	fs := flag.NewFlagSet("purge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	raw := fs.Bool("raw", false, "select opted-in raw command logs")
	olderThan := fs.Duration("older-than", 7*24*time.Hour, "minimum age, for example 168h")
	confirm := fs.Bool("confirm", false, "delete selected files; otherwise preview only")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if !*raw {
		return 2, errors.New("purge currently requires --raw")
	}
	if *olderThan < 0 {
		return 2, errors.New("--older-than cannot be negative")
	}
	cwd, _ := os.Getwd()
	root, err := gitx.Root(cwd)
	if err != nil {
		return classify(err), err
	}
	result := purge.Raw(root, purge.Options{OlderThan: *olderThan, Confirm: *confirm})
	fmt.Fprintf(os.Stdout, "Raw evidence selected: %d; deleted: %d; failed: %d\n", result.Selected, result.Deleted, result.Failed)
	if !*confirm {
		fmt.Fprintln(os.Stdout, "Preview only. Re-run with --confirm to delete the selected raw files.")
	}
	if result.Failed > 0 {
		return 3, errors.New("one or more raw evidence files could not be processed")
	}
	return 0, nil
}

func initCommand(args []string) (int, error) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("force", false, "replace existing configuration")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	cwd, _ := os.Getwd()
	root, err := gitx.Root(cwd)
	if err != nil {
		return classify(err), err
	}
	if err := config.Init(root, *force); err != nil {
		return classify(err), err
	}
	fmt.Fprintln(os.Stdout, "AgentProof initialized in", root)
	return 0, nil
}

func recordCommand(args []string) (int, error) {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	objective := fs.String("objective", "", "original objective given to the coding agent")
	agent := fs.String("agent", "codex", "session adapter: codex or claude")
	model := fs.String("model", "", "model identifier when known")
	retainRaw := fs.Bool("retain-raw", false, "retain raw command output locally for this run")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	cwd, _ := os.Getwd()
	run, err := record.Run(cwd, record.Options{Objective: *objective, Agent: *agent, Model: *model, Command: fs.Args(), RetainRaw: *retainRaw})
	if run.RunID != "" {
		fmt.Fprintf(os.Stdout, "\nRecorded run %s (%d files changed)\n", run.RunID, len(run.Repository.Changes))
	}
	if err != nil {
		// The recorded agent's own exit code is evidence, not AgentProof's
		// verdict: forwarding it would let a child's 2 be read as invalid
		// AgentProof usage and a child's 3 as an internal failure.
		if apperr.IsUsage(err) {
			return 2, err
		}
		return 1, err
	}
	return 0, nil
}

func verifyCommand(args []string) (int, error) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	base := fs.String("base", "", "Git baseline when no recorded session is available")
	var testResults stringList
	fs.Var(&testResults, "test-result", "repository-relative JUnit XML or Go test2json artifact; repeatable")
	requireTests := fs.Bool("require-tests", false, "fail when no valid test evidence is supplied")
	failOn := fs.String("fail-on", "", "CI threshold: critical, high, medium, low, or none")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if *failOn != "" && !validThreshold(*failOn) {
		return 2, errors.New("--fail-on must be critical, high, medium, low, or none")
	}
	cwd, _ := os.Getwd()
	result, err := verify.Run(cwd, verify.Options{Base: *base, TestResults: testResults, RequireTests: *requireTests, FailOn: *failOn})
	if err != nil {
		return classify(err), err
	}
	fmt.Fprintf(os.Stdout, "AgentProof verification: %s\n", strings.ToUpper(result.Run.Status))
	fmt.Fprintf(os.Stdout, "Report: .agentproof/report.md\nBundle ID: %s\n", result.BundleID)
	return result.ExitCode, nil
}

// Exit codes are a public contract CI systems branch on, so the boundary
// classifies once: a fixable invocation is never reported as an AgentProof
// internal failure.
func classify(err error) int {
	if apperr.IsUsage(err) {
		return 2
	}
	return 3
}

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func validThreshold(value string) bool {
	switch value {
	case "critical", "high", "medium", "low", "none":
		return true
	default:
		return false
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `AgentProof — know exactly what your coding agent changed.

Usage:
  agentproof init
  agentproof record --objective "Add session rotation" --agent codex -- codex
  agentproof verify --test-result test-results.jsonl [--require-tests]
  agentproof verify --base origin/main
  agentproof purge --raw --older-than 168h [--confirm]

Commands:
  init      Create a local-first AgentProof configuration
              --force                 replace an existing configuration
  record    Record an agent command and its Git change window
              --objective <text>      objective given to the coding agent
              --agent <name>          session adapter: codex or claude
              --model <id>            model identifier when known
              --retain-raw            retain raw command output locally
  verify    Ingest evidence and generate deterministic integrity reports
              --base <ref>            Git baseline when no session was recorded
              --test-result <path>    JUnit XML or Go test2json artifact; repeatable
              --require-tests         fail when no valid test evidence is supplied
              --fail-on <severity>    critical, high, medium, low, or none
  purge     Preview or delete opted-in raw command logs
              --raw                   select opted-in raw command logs
              --older-than <dur>      minimum age, for example 168h
              --confirm               delete; otherwise preview only

Exit codes:
  0  verification passed, or passed with warnings only
  1  required evidence was missing, or a policy threshold was met
  2  invalid usage or invalid local configuration
  3  an adapter, analyzer, or internal step failed

A recorded agent's own exit code is evidence inside the report; it never
becomes AgentProof's exit code.`)
}

func runsCommand(_ []string) (int, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return 3, err
	}
	runs, err := status.ListRuns(cwd)
	if err != nil {
		return 3, err
	}
	if len(runs) == 0 {
		fmt.Fprintln(os.Stdout, "No runs recorded.")
		return 0, nil
	}
	fmt.Fprintf(os.Stdout, "%-36s  %-12s  %-10s  %s\n", "ID", "STATE", "AGENT", "OBJECTIVE")
	for _, r := range runs {
		fmt.Fprintf(os.Stdout, "%-36s  %-12s  %-10s  %s\n", r.ID, r.State, r.Agent, r.Objective)
	}
	return 0, nil
}
