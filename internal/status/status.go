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

	evPath := filepath.Join(root, config.DirName, "evidence.json")
	if data, err := os.ReadFile(evPath); err == nil {
		var ev struct {
			Run struct {
				Status   string    `json:"status"`
				BundleID string    `json:"bundleID"`
				At       time.Time `json:"at"`
			} `json:"run"`
		}
		if json.Unmarshal(data, &ev) == nil {
			s.LastStatus = ev.Run.Status
			s.LastBundleID = ev.Run.BundleID
			s.LastVerifiedAt = ev.Run.At
		}
	}

	return s, nil
}
