package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
)

// State summarises the AgentProof state for a project root.
type State struct {
	Initialized   bool
	RunCount      int
	AbandonedRuns int
	LastBundleID  string
	LastStatus    string // "passed", "warning", "failed", or ""
	LastVerifiedAt time.Time
}

// Read loads the AgentProof state from root. A missing config.json is not an
// error — it simply means the project is not yet initialised.
func Read(root string) (State, error) {
	var s State
	cfgPath := filepath.Join(root, config.DirName, "config.json")
	if _, err := os.Stat(cfgPath); err != nil {
		return s, nil // not initialised
	}
	s.Initialized = true

	runsDir := filepath.Join(root, config.DirName, "runs")
	entries, _ := os.ReadDir(runsDir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s.RunCount++
		stateFile := filepath.Join(runsDir, e.Name(), "state.json")
		if data, err := os.ReadFile(stateFile); err == nil {
			var rs struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(data, &rs) == nil && rs.Status == "abandoned" {
				s.AbandonedRuns++
			}
		}
	}

	// evidence.json is the normalized evidence.Run emitted by verify: status and
	// bundle_id live at the document root, not under a nested "run" object.
	evPath := filepath.Join(root, config.DirName, "evidence.json")
	if data, err := os.ReadFile(evPath); err == nil {
		var ev struct {
			Status   string `json:"status"`
			BundleID string `json:"bundle_id"`
		}
		if json.Unmarshal(data, &ev) == nil {
			s.LastStatus = ev.Status
			s.LastBundleID = ev.BundleID
		}
	}

	// LastVerifiedAt is the instant verification produced the current bundle.
	// evidence.json cannot provide it: its StartedAt/FinishedAt describe the
	// recorded agent window (and for --base runs they are the verification
	// instant by construction), so the timestamp comes from attestation.json,
	// which verify writes atomically in the same publication step.
	attPath := filepath.Join(root, config.DirName, "attestation.json")
	if data, err := os.ReadFile(attPath); err == nil {
		var att struct {
			CreatedAt time.Time `json:"created_at"`
		}
		if json.Unmarshal(data, &att) == nil {
			s.LastVerifiedAt = att.CreatedAt
		}
	}

	return s, nil
}

// RunSummary holds the key fields for a single recorded run.
type RunSummary struct {
	ID        string
	Objective string
	Agent     string
	StartedAt time.Time
	State     string // "recorded", "complete", "abandoned", or "recording"
}

// ListRuns returns a summary of every run directory under root. A missing runs
// directory is not an error — it simply returns nil.
func ListRuns(root string) ([]RunSummary, error) {
	runsDir := filepath.Join(root, config.DirName, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var result []RunSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rs := RunSummary{ID: e.Name()}
		// record.go writes record.json with objective, agent, startedAt, status.
		recordFile := filepath.Join(runsDir, e.Name(), "record.json")
		if data, err := os.ReadFile(recordFile); err == nil {
			var rec struct {
				Objective string    `json:"objective"`
				Agent     string    `json:"agent"`
				StartedAt time.Time `json:"startedAt"`
				Status    string    `json:"status"`
			}
			if json.Unmarshal(data, &rec) == nil {
				rs.Objective = rec.Objective
				rs.Agent = rec.Agent
				rs.StartedAt = rec.StartedAt
				rs.State = rec.Status
			}
		}
		result = append(result, rs)
	}
	return result, nil
}
