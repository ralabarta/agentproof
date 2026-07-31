package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/evidence"
	"github.com/ralabarta/agentproof/internal/gitx"
	"github.com/ralabarta/agentproof/internal/safefile"
	"github.com/ralabarta/agentproof/internal/scan"
	"github.com/ralabarta/agentproof/internal/session"
)

type Options struct {
	Objective string
	Agent     string
	Model     string
	Command   []string
	RetainRaw bool
}

func Run(cwd string, opts Options) (evidence.Run, error) {
	if len(opts.Command) == 0 {
		return evidence.Run{}, errors.New("record requires a command after --")
	}
	root, err := gitx.Root(cwd)
	if err != nil {
		return evidence.Run{}, err
	}
	if _, err := config.Load(root); err != nil {
		return evidence.Run{}, errors.New("AgentProof is not initialized; run agentproof init")
	}
	start, err := gitx.TakeSnapshot(root)
	if err != nil {
		return evidence.Run{}, err
	}
	started := time.Now().UTC()
	runID := started.Format("20060102T150405.000000000Z")
	runDir := filepath.Join(root, config.DirName, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return evidence.Run{}, err
	}
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	var logFile *os.File
	if opts.RetainRaw {
		logFile, err = os.OpenFile(filepath.Join(runDir, "command.log"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return evidence.Run{}, err
		}
		cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
		cmd.Stderr = io.MultiWriter(os.Stderr, logFile)
	}
	runErr := cmd.Run()
	if logFile != nil {
		_ = logFile.Close()
	}
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 127
		}
	}
	finished := time.Now().UTC()
	end, err := gitx.TakeSnapshot(root)
	if err != nil {
		return evidence.Run{}, err
	}
	repo, patch, err := gitx.Collect(root, start, end)
	if err != nil {
		return evidence.Run{}, err
	}
	findings := scan.Run(repo.Changes, patch)
	redactedPatch, redactionCount := scan.RedactPatch(patch)
	if err := safefile.Write(filepath.Join(runDir, "changes.patch"), []byte(redactedPatch), 0o600); err != nil {
		return evidence.Run{}, err
	}
	redactedCommand := make([]string, len(opts.Command))
	for i, value := range opts.Command {
		redactedCommand[i] = scan.RedactString(value)
	}
	result := evidence.Run{
		SchemaVersion: evidence.RunSchemaVersion,
		RunID:         runID,
		Objective:     scan.RedactString(strings.TrimSpace(opts.Objective)),
		Agent:         opts.Agent,
		Model:         opts.Model,
		Command:       redactedCommand,
		StartedAt:     started,
		FinishedAt:    finished,
		DurationMS:    finished.Sub(started).Milliseconds(),
		ExitCode:      exitCode,
		Repository:    repo,
		Findings:      findings,
		Status:        "recorded",
		Metadata: map[string]string{
			"raw_retained":    fmt.Sprintf("%t", opts.RetainRaw),
			"redaction_count": fmt.Sprintf("%d", redactionCount),
		},
		Claims: []evidence.Claim{{
			Type: "git-association", Statement: "Git changes were observed within the command window; this does not establish causality or authorship.",
			Confidence: confidence(repo.DirtyBefore), Evidence: "git snapshots and changes.patch",
		}},
	}
	if result.Objective == "" {
		result.Objective = "Not provided"
	}
	for _, native := range session.Discover(opts.Agent, started) {
		summary, summaryErr := session.Summarize(opts.Agent, native)
		if summaryErr == nil {
			result.Sessions = append(result.Sessions, summary)
			if summary.State == evidence.Observed {
				result.Usage.InputTokens += summary.Usage.InputTokens
				result.Usage.OutputTokens += summary.Usage.OutputTokens
				result.Usage.CachedTokens += summary.Usage.CachedTokens
			}
		}
	}
	if err := save(filepath.Join(runDir, "record.json"), result); err != nil {
		return evidence.Run{}, err
	}
	latest := map[string]string{"run_id": runID, "record": filepath.ToSlash(filepath.Join("runs", runID, "record.json"))}
	if err := save(filepath.Join(root, config.DirName, "latest.json"), latest); err != nil {
		return evidence.Run{}, err
	}
	if runErr != nil {
		return result, fmt.Errorf("recorded command exited with code %d", exitCode)
	}
	return result, nil
}

func confidence(dirty bool) string {
	if dirty {
		return "inferred"
	}
	return "derived"
}

func save(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return safefile.Write(path, append(b, '\n'), 0o600)
}
