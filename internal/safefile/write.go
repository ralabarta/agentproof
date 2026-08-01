package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Contained reports whether full stays inside root once the filesystem, not
// just the string, is consulted. A lexical containment check answers a question
// about the path text; rejecting a symlinked final component is not enough
// either, because an intermediate directory symlink keeps every component
// inside root while the bytes actually opened come from anywhere on the machine.
//
// A path that does not exist cannot leak content and must stay reportable as
// missing evidence rather than as an escape, so only its parent is required to
// be contained.
func Contained(root, full string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return errors.New("cannot resolve containment root")
	}
	resolved, err := filepath.EvalSymlinks(full)
	if errors.Is(err, os.ErrNotExist) {
		resolved, err = filepath.EvalSymlinks(filepath.Dir(full))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	if err != nil {
		return errors.New("cannot resolve path")
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("path escapes its permitted root")
	}
	return nil
}

func Write(path string, data []byte, permission os.FileMode) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to replace a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".agentproof-write-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(permission); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
