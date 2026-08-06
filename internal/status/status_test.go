package status_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestListRuns(t *testing.T) {
	dir := t.TempDir()

	// no .agentproof/runs dir → empty slice, no error
	runs, err := status.ListRuns(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}

	// create a run dir with record.json (matches what record.go writes)
	runDir := filepath.Join(dir, ".agentproof", "runs", "run-abc")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	recordData, _ := json.Marshal(map[string]interface{}{
		"status":    "recorded",
		"startedAt": now,
		"objective": "fix auth",
		"agent":     "codex",
	})
	if err := os.WriteFile(filepath.Join(runDir, "record.json"), recordData, 0o600); err != nil {
		t.Fatal(err)
	}

	runs, err = status.ListRuns(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "run-abc" {
		t.Fatalf("expected ID 'run-abc', got %q", runs[0].ID)
	}
	if runs[0].Objective != "fix auth" {
		t.Fatalf("expected objective 'fix auth', got %q", runs[0].Objective)
	}
	if runs[0].Agent != "codex" {
		t.Fatalf("expected agent 'codex', got %q", runs[0].Agent)
	}
	if runs[0].State != "recorded" {
		t.Fatalf("expected state 'recorded', got %q", runs[0].State)
	}
}
