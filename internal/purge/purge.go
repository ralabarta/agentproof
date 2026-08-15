package purge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
	"github.com/ralabarta/agentproof/internal/record"
	"github.com/ralabarta/agentproof/internal/safefile"
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
	return runs(root, opts, nil)
}

func runs(root string, opts Options, beforeDelete func(string)) Result {
	result := Result{}
	runs := filepath.Join(root, config.DirName, "runs")
	metadataInfo, runsInfo, err := validateRunsParent(root, runs)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			result.Failed++
		}
		return result
	}
	cutoff := time.Now().Add(-opts.OlderThan)
	entries, err := os.ReadDir(runs)
	if err != nil {
		result.Failed++
		return result
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		dir := filepath.Join(runs, entry.Name())
		dead, stateErr := deadRun(root, dir)
		if stateErr != nil {
			result.Failed++
			continue
		}
		if !dead {
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
		if beforeDelete != nil {
			beforeDelete(dir)
		}
		if err := validateRunForDelete(root, runs, dir, metadataInfo, runsInfo, info); err != nil {
			result.Failed++
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

func validateRunsParent(root, runs string) (os.FileInfo, os.FileInfo, error) {
	metadata := filepath.Join(root, config.DirName)
	metadataInfo, err := os.Lstat(metadata)
	if err != nil {
		return nil, nil, err
	}
	if !metadataInfo.IsDir() || metadataInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("metadata path is not a real directory")
	}
	runsInfo, err := os.Lstat(runs)
	if err != nil {
		return nil, nil, err
	}
	if !runsInfo.IsDir() || runsInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("runs path is not a real directory")
	}
	if err := safefile.Contained(metadata, runs); err != nil {
		return nil, nil, err
	}
	return metadataInfo, runsInfo, nil
}

func validateRunForDelete(root, runs, dir string, originalMetadata, originalRuns, originalRun os.FileInfo) error {
	if filepath.Dir(dir) != runs || filepath.Base(dir) == "." {
		return errors.New("run is not a direct child")
	}
	metadataInfo, runsInfo, err := validateRunsParent(root, runs)
	if err != nil {
		return err
	}
	if !os.SameFile(originalMetadata, metadataInfo) || !os.SameFile(originalRuns, runsInfo) {
		return errors.New("run parent identity changed")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("run path is not a real directory")
	}
	if !os.SameFile(originalRun, info) {
		return errors.New("run identity changed")
	}
	// These identity and containment checks reject replacements present at this
	// final boundary. Active same-UID replacement after this final check remains
	// a residual race and is out of scope; descriptor-relative resistance is not claimed.
	return nil
}

// deadRun reports whether a run directory will never produce evidence. A
// "recording" run is dead unless it matches a valid live lock owner. An active
// lock with malformed metadata fails closed because its owner cannot be proven.
func deadRun(root, dir string) (bool, error) {
	state, err := readState(dir)
	if err != nil {
		return false, err
	}
	switch state.Status {
	case "abandoned":
		return true, nil
	case "recording":
		lockStatus, err := record.RecordLockStatus(root)
		if err != nil {
			return false, nil
		}
		if !lockStatus.Active {
			return true, nil
		}
		if lockStatus.Metadata == nil {
			return false, nil
		}
		return lockStatus.Metadata.RunID != filepath.Base(dir), nil
	default:
		return false, nil
	}
}
