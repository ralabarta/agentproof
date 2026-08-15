package purge

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/record"
)

type Options struct {
	OlderThan time.Duration
	Confirm   bool
	Runs      bool
}

type Result struct {
	Selected int
	Deleted  int
	Failed   int
}

func Raw(root string, opts Options) Result {
	result := Result{}
	runs := filepath.Join(root, config.DirName, "runs")
	cutoff := time.Now().Add(-opts.OlderThan)
	_ = filepath.WalkDir(runs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.Failed++
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Name() != "command.log" {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			result.Failed++
			return nil
		}
		if opts.OlderThan > 0 && info.ModTime().After(cutoff) {
			return nil
		}
		result.Selected++
		if !opts.Confirm {
			return nil
		}
		if removeErr := os.Remove(path); removeErr != nil {
			result.Failed++
		} else {
			result.Deleted++
		}
		return nil
	})
	return result
}

// Runs applies a retention policy to run directories. Only runs that can never
// produce evidence are selected — abandoned by a signal, or left in the
// recording state by a crash that bypassed signal handling (SIGKILL, power
// loss). A record in progress and any run without a lifecycle state file are
// never touched, so completed evidence survives.
func Runs(root string, opts Options) Result {
	result := Result{}
	runs := filepath.Join(root, config.DirName, "runs")
	cutoff := time.Now().Add(-opts.OlderThan)
	entries, err := os.ReadDir(runs)
	if err != nil {
		if !os.IsNotExist(err) {
			result.Failed++
		}
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		dir := filepath.Join(runs, entry.Name())
		if !deadRun(root, dir) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			result.Failed++
			continue
		}
		if opts.OlderThan > 0 && info.ModTime().After(cutoff) {
			continue
		}
		result.Selected++
		if !opts.Confirm {
			continue
		}
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			result.Failed++
		} else {
			result.Deleted++
		}
	}
	return result
}

// deadRun reports whether a run directory will never produce evidence. A
// "recording" run is dead unless it matches a valid live lock owner. An active
// lock with malformed metadata fails closed because its owner cannot be proven.
func deadRun(root, dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return false
	}
	var state struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(data, &state) != nil {
		return false
	}
	switch state.Status {
	case "abandoned":
		return true
	case "recording":
		lockStatus, err := record.RecordLockStatus(root)
		if err != nil {
			return false
		}
		if !lockStatus.Active {
			return true
		}
		if lockStatus.Metadata == nil {
			return false
		}
		return lockStatus.Metadata.RunID != filepath.Base(dir)
	default:
		return false
	}
}
