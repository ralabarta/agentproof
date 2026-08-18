package purge

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const maxStateSize = 64 << 10

type lifecycleState struct {
	Status string `json:"status"`
}

// ReadStateStatus safely reads the lifecycle status for a run directory.
func ReadStateStatus(dir string) (string, error) {
	state, err := readState(dir)
	if err != nil {
		return "", err
	}
	return state.Status, nil
}

func readState(dir string) (lifecycleState, error) {
	path := filepath.Join(dir, "state.json")
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return lifecycleState{}, err
	}
	if !pathInfo.Mode().IsRegular() {
		return lifecycleState{}, errors.New("state is not a regular file")
	}
	file, err := openStateFile(path)
	if err != nil {
		return lifecycleState{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return lifecycleState{}, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return lifecycleState{}, errors.New("state changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxStateSize+1))
	if err != nil {
		return lifecycleState{}, err
	}
	if len(data) > maxStateSize {
		return lifecycleState{}, errors.New("state exceeds size limit")
	}
	var state lifecycleState
	if err := json.Unmarshal(data, &state); err != nil {
		return lifecycleState{}, err
	}
	return state, nil
}
