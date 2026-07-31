package purge

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
)

type Options struct {
	OlderThan time.Duration
	Confirm   bool
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
