package status_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralabarta/agentproof/internal/status"
)

func TestReadStatus(t *testing.T) {
	dir := t.TempDir()

	// 1. Uninitialized: no config.json → Initialized == false, no error
	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error on uninitialized dir: %v", err)
	}
	if s.Initialized {
		t.Fatal("expected Initialized=false for uninitialized dir")
	}

	// 2. Create config.json → Initialized == true, RunCount == 0
	apDir := filepath.Join(dir, ".agentproof")
	if err := os.MkdirAll(filepath.Join(apDir, "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error after init: %v", err)
	}
	if !s.Initialized {
		t.Fatal("expected Initialized=true after config.json created")
	}
	if s.RunCount != 0 {
		t.Fatalf("expected RunCount=0, got %d", s.RunCount)
	}

	// 3. Create one abandoned run dir
	runDir := filepath.Join(apDir, "runs", "run-abc123")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stateData, _ := json.Marshal(map[string]string{"status": "abandoned"})
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err = status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error with run: %v", err)
	}
	if s.RunCount != 1 {
		t.Fatalf("expected RunCount=1, got %d", s.RunCount)
	}
	if s.AbandonedRuns != 1 {
		t.Fatalf("expected AbandonedRuns=1, got %d", s.AbandonedRuns)
	}
}
