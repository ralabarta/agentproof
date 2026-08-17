package record

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ralabarta/agentproof/internal/apperr"
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

const (
	abandonedStatePublicationDiagnostic = "agentproof: abandoned-state publication failed"
	processSignalNotRunningDiagnostic   = "agentproof: signal forwarding failed: child process is not running"
	processSignalUnsupportedDiagnostic  = "agentproof: signal forwarding failed: unsupported signal or platform"
)

func Run(cwd string, opts Options) (evidence.Run, error) {
	if len(opts.Command) == 0 {
		return evidence.Run{}, fmt.Errorf("%w: record requires a command after --", apperr.ErrUsage)
	}
	root, err := gitx.Root(cwd)
	if err != nil {
		return evidence.Run{}, err
	}
	if _, err := config.Load(root); err != nil {
		return evidence.Run{}, fmt.Errorf("%w: AgentProof is not initialized; run agentproof init", apperr.ErrUsage)
	}
	started := time.Now().UTC()
	runID := started.Format("20060102T150405.000000000Z")
	lock, err := acquireRecordLock(root, runID)
	if err != nil {
		return evidence.Run{}, err
	}
	defer lock.Release()
	start, err := gitx.TakeSnapshot(root)
	if err != nil {
		return evidence.Run{}, err
	}
	if err := validateRecordRoot(filepath.Join(root, config.DirName)); err != nil {
		return evidence.Run{}, err
	}
	runDir := filepath.Join(root, config.DirName, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return evidence.Run{}, err
	}
	statePath := filepath.Join(runDir, "state.json")

	// The lifecycle state file is how status and doctor detect interrupted
	// runs. The handler is armed before recording is published so observing that
	// state guarantees interrupt handling is active.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	lifecycle := newRunLifecycle(statePath, started, lifecycleDependencies{})
	lifecycle.startHandler(signals)
	handlerStopped := false
	defer func() {
		if !handlerStopped {
			lifecycle.stopHandler(func() { signal.Stop(signals) })
		}
	}()
	if err := lifecycle.publishRecording(); err != nil {
		return evidence.Run{}, err
	}
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	configureProcess(cmd)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	var logFile *os.File
	var logWriter io.WriteCloser
	if opts.RetainRaw {
		logFile, err = os.OpenFile(filepath.Join(runDir, "command.log"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return evidence.Run{}, err
		}
		// The recorded process is not under AgentProof's control, so the same
		// secret rules that redact changes.patch apply to what it prints. Only
		// the retained file is wrapped: the operator's terminal keeps the raw
		// bytes it would have seen without AgentProof.
		logWriter = scan.NewRedactingWriter(logFile)
		cmd.Stdout = io.MultiWriter(os.Stdout, logWriter)
		cmd.Stderr = io.MultiWriter(os.Stderr, logWriter)
	}
	runErr := lifecycle.startCommand(cmd.Start, func() *os.Process { return cmd.Process })
	if runErr == nil {
		runErr = lifecycle.waitAndReap(cmd.Wait)
	}
	lifecycle.stopHandler(func() { signal.Stop(signals) })
	handlerStopped = true
	if logWriter != nil {
		_ = logWriter.Close()
	}
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
	if err := lifecycle.publishComplete(runState{Status: "complete", StartedAt: started, CompletedAt: &finished}); err != nil {
		return evidence.Run{}, err
	}
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

// runState is the per-run lifecycle marker written by record and read by
// status and doctor to detect abandoned runs. Keys are camelCase to match the
// published contract; only the status key is consumed today, but the rest is
// kept stable so the file stays self-describing. CompletedAt is a pointer
// because time.Time never counts as empty for omitempty, which would leak a
// zero timestamp into the recording state.
type runState struct {
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Signal      string     `json:"signal,omitempty"`
}

func writeRunState(path string, state runState) error {
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return safefile.Write(path, append(b, '\n'), 0o600)
}

func processSignalForwardingDiagnostic(result processSignalForwarding) string {
	switch result {
	case processSignalForwarded:
		return ""
	case processSignalNotRunning:
		return processSignalNotRunningDiagnostic
	default:
		return processSignalUnsupportedDiagnostic
	}
}

func signalName(s os.Signal) string {
	switch s {
	case os.Interrupt:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		if sig, ok := s.(syscall.Signal); ok {
			return strconv.Itoa(int(sig))
		}
		return s.String()
	}
}

// LiveRecord reports whether a record currently owns the repository's kernel
// lock. Retained metadata is diagnostic only and never makes an idle lock live.
func LiveRecord(root string) bool {
	status, err := statusRecordLock(root)
	return err == nil && status.Active
}

func confidence(dirty bool) evidence.ClaimConfidence {
	if dirty {
		return evidence.ConfidenceInferred
	}
	return evidence.ConfidenceDerived
}

func save(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return safefile.Write(path, append(b, '\n'), 0o600)
}
