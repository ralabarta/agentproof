package verify

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/ralabarta/agentproof/internal/config"
)

// LoadRunPointer reads .agentproof/latest.json under the same conditions
// verify applies to it: an absent pointer is not an error (verify maps that
// case to its usage hint and a fresh init legitimately has none), while a
// pointer that exists but does not carry a record path is invalid and both
// callers must disagree on nothing about that fact, so the check lives here.
//
// found is false when the file is absent. When found is true, invalid is
// non-nil exactly when the pointer does not name a record run.
func LoadRunPointer(cwd string) (latest map[string]string, found bool, invalid error) {
	b, err := os.ReadFile(filepath.Join(cwd, config.DirName, "latest.json"))
	if err != nil {
		return nil, false, nil
	}
	var parsed map[string]string
	if json.Unmarshal(b, &parsed) != nil || parsed["record"] == "" {
		return nil, true, errors.New("invalid .agentproof/latest.json")
	}
	return parsed, true, nil
}
