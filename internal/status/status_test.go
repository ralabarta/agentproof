package status_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/evidence"
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

// TestReadPopulatesVerificationFields guards the evidence.json contract: the
// JSON is built with the same types and marshalling that verify.Run emits, so a
// schema drift in the evidence or attestation documents fails here instead of
// silently leaving status fields empty.
func TestReadPopulatesVerificationFields(t *testing.T) {
	dir := initializedDir(t)
	apDir := filepath.Join(dir, ".agentproof")

	verifiedAt := time.Date(2026, 8, 6, 14, 30, 0, 0, time.UTC)
	evidenceBytes, err := json.MarshalIndent(evidence.Run{
		SchemaVersion: evidence.RunSchemaVersion,
		RunID:         "git-20260806T143000Z",
		Status:        "warning",
		BundleID:      "7f0c9a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e",
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "evidence.json"), append(evidenceBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	attestationBytes, err := json.MarshalIndent(evidence.Attestation{
		SchemaVersion: "agentproof.dev/attestation/v1",
		Algorithm:     "sha256",
		BundleID:      "7f0c9a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e",
		CreatedAt:     verifiedAt,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "attestation.json"), append(attestationBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.Initialized {
		t.Fatal("expected Initialized=true")
	}
	if s.LastStatus != "warning" {
		t.Fatalf("expected LastStatus %q, got %q", "warning", s.LastStatus)
	}
	if s.LastBundleID != "7f0c9a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e" {
		t.Fatalf("expected LastBundleID to be populated, got %q", s.LastBundleID)
	}
	if !s.LastVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("expected LastVerifiedAt %v, got %v", verifiedAt, s.LastVerifiedAt)
	}
}

// TestReadWithoutAttestation verifies graceful degradation: evidence.json can
// exist without its attestation (a crash between the two atomic writes), in
// which case the bundle identity and status still load and the timestamp stays
// zero instead of erroring.
func TestReadWithoutAttestation(t *testing.T) {
	dir := initializedDir(t)
	apDir := filepath.Join(dir, ".agentproof")
	evidenceBytes, err := json.MarshalIndent(evidence.Run{Status: "passed", BundleID: "deadbeef"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "evidence.json"), append(evidenceBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.LastStatus != "passed" || s.LastBundleID != "deadbeef" {
		t.Fatalf("expected status/bundle to load without attestation, got %q / %q", s.LastStatus, s.LastBundleID)
	}
	if !s.LastVerifiedAt.IsZero() {
		t.Fatalf("expected zero LastVerifiedAt without attestation, got %v", s.LastVerifiedAt)
	}
}

// TestReadIgnoresMalformedEvidence ensures a corrupt evidence.json degrades to
// empty fields rather than an error, matching the prior contract of the command.
func TestReadIgnoresMalformedEvidence(t *testing.T) {
	dir := initializedDir(t)
	apDir := filepath.Join(dir, ".agentproof")
	if err := os.WriteFile(filepath.Join(apDir, "evidence.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.LastStatus != "" || s.LastBundleID != "" || !s.LastVerifiedAt.IsZero() {
		t.Fatalf("expected empty verification fields for malformed evidence, got %q / %q / %v", s.LastStatus, s.LastBundleID, s.LastVerifiedAt)
	}
}

// initializedDir returns a temp directory with a config.json, mirroring what
// agentproof init produces.
func initializedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	apDir := filepath.Join(dir, ".agentproof")
	if err := os.MkdirAll(filepath.Join(apDir, "runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
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
