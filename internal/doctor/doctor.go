package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/record"
	"github.com/ralabarta/agentproof/internal/status"
)

// Severity classifies the impact of a finding.
type Severity string

const (
	SeverityOK    Severity = "ok"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

// Finding is a single diagnostic result.
type Finding struct {
	Name     string
	Severity Severity
	Detail   string
}

// Report aggregates all findings for a project root.
type Report struct {
	Findings []Finding
	Healthy  bool
}

// Run performs all diagnostic checks against cwd and returns a Report.
func Run(cwd string) (Report, error) {
	var r Report

	// check 1: git binary
	if _, err := exec.LookPath("git"); err != nil {
		r.Findings = append(r.Findings, Finding{
			Name:     "git-binary",
			Severity: SeverityError,
			Detail:   "git not found in PATH — required for all agentproof operations",
		})
	} else {
		r.Findings = append(r.Findings, Finding{Name: "git-binary", Severity: SeverityOK})
	}

	// check 2: agentproof initialized
	s, err := status.Read(cwd)
	if err != nil {
		return r, err
	}
	if !s.Initialized {
		r.Findings = append(r.Findings, Finding{
			Name:     "agentproof-init",
			Severity: SeverityWarn,
			Detail:   "not initialized — run `agentproof init`",
		})
	} else {
		r.Findings = append(r.Findings, Finding{Name: "agentproof-init", Severity: SeverityOK})
		// check 3: abandoned runs
		if s.AbandonedRuns > 0 {
			r.Findings = append(r.Findings, Finding{
				Name:     "abandoned-runs",
				Severity: SeverityWarn,
				Detail:   fmt.Sprintf("%d abandoned run(s) — consider running `agentproof purge`", s.AbandonedRuns),
			})
		}

		// check 4: runs stuck in the recording state. A crash that bypasses
		// signal handling (SIGKILL, power loss, a panic) leaves state.json as
		// "recording" forever, indistinguishable from a live run except that a
		// live record still owns a live lock.
		stuck := s.RecordingRuns
		if matchingLiveRecordingRun(cwd) && stuck > 0 {
			stuck--
		}
		if stuck > 0 {
			r.Findings = append(r.Findings, Finding{
				Name:     "stuck-recording-runs",
				Severity: SeverityWarn,
				Detail:   fmt.Sprintf("%d run(s) stuck in the recording state — the record process died without completing; consider purging them", stuck),
			})
		}
	}

	// check 5: go toolchain
	if _, err := exec.LookPath("go"); err != nil {
		r.Findings = append(r.Findings, Finding{
			Name:     "go-toolchain",
			Severity: SeverityWarn,
			Detail:   "go not found in PATH — required for Go coverage ingestion",
		})
	} else {
		r.Findings = append(r.Findings, Finding{Name: "go-toolchain", Severity: SeverityOK})
	}

	// healthy = no SeverityError findings
	r.Healthy = true
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			r.Healthy = false
			break
		}
	}
	return r, nil
}

func matchingLiveRecordingRun(root string) bool {
	lockStatus, err := record.RecordLockStatus(root)
	if err != nil || !lockStatus.Active || lockStatus.Metadata == nil {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(root, config.DirName, "runs"))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() != lockStatus.Metadata.RunID {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, config.DirName, "runs", entry.Name(), "state.json"))
		if err != nil {
			return false
		}
		var state struct {
			Status string `json:"status"`
		}
		return json.Unmarshal(data, &state) == nil && state.Status == "recording"
	}
	return false
}
