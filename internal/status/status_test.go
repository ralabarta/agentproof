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

	// 3. Create an abandoned run and one stuck in the recording state
	for _, state := range []string{"abandoned", "recording"} {
		runDir := filepath.Join(apDir, "runs", "run-"+state)
		if err := os.MkdirAll(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		stateData, _ := json.Marshal(map[string]string{"status": state})
		if err := os.WriteFile(filepath.Join(runDir, "state.json"), stateData, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err = status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error with runs: %v", err)
	}
	if s.RunCount != 2 {
		t.Fatalf("expected RunCount=2, got %d", s.RunCount)
	}
	if s.AbandonedRuns != 1 {
		t.Fatalf("expected AbandonedRuns=1, got %d", s.AbandonedRuns)
	}
	if s.RecordingRuns != 1 {
		t.Fatalf("expected RecordingRuns=1, got %d", s.RecordingRuns)
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

func TestReadCorrelatesEvidenceAndAttestationBundleIDs(t *testing.T) {
	verifiedAt := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	dir := assertMismatchedVerification(t, verifiedAt)

	writeVerificationFiles(t, filepath.Join(dir, ".agentproof"), "bundle-a", "bundle-a", verifiedAt)
	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.LastVerifiedAt.Equal(verifiedAt) {
		t.Fatalf("expected LastVerifiedAt %v for matching bundles, got %v", verifiedAt, s.LastVerifiedAt)
	}
}

func TestReadSuppressesTimestampForMismatchedBundle(t *testing.T) {
	assertMismatchedVerification(t, time.Date(2026, 8, 15, 11, 0, 0, 0, time.UTC))
}

func assertMismatchedVerification(t *testing.T, verifiedAt time.Time) string {
	t.Helper()
	dir := initializedDir(t)
	writeVerificationFiles(t, filepath.Join(dir, ".agentproof"), "bundle-a", "bundle-b", verifiedAt)

	s, err := status.Read(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.LastStatus != "passed" || s.LastBundleID != "bundle-a" {
		t.Fatalf("expected evidence-sourced status/bundle, got %q / %q", s.LastStatus, s.LastBundleID)
	}
	if !s.LastVerifiedAt.IsZero() {
		t.Fatalf("expected zero LastVerifiedAt for mismatched bundles, got %v", s.LastVerifiedAt)
	}
	return dir
}

func TestReadAcceptsMatchingBundleGeneration(t *testing.T) {
	verifiedAt := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		evidenceBundleID string
		attBundleID      string
		attestation      string
		wantTimestamp    bool
	}{
		{name: "matching nonempty bundle IDs", evidenceBundleID: "bundle-a", attBundleID: "bundle-a", wantTimestamp: true},
		{name: "empty evidence bundle ID", attBundleID: "bundle-a"},
		{name: "empty attestation bundle ID", evidenceBundleID: "bundle-a"},
		{name: "both bundle IDs empty"},
		{name: "missing attestation", evidenceBundleID: "bundle-a", attestation: "missing"},
		{name: "malformed attestation", evidenceBundleID: "bundle-a", attestation: "malformed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initializedDir(t)
			apDir := filepath.Join(dir, ".agentproof")
			writeVerificationFiles(t, apDir, tt.evidenceBundleID, tt.attBundleID, verifiedAt)
			switch tt.attestation {
			case "missing":
				if err := os.Remove(filepath.Join(apDir, "attestation.json")); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				if err := os.WriteFile(filepath.Join(apDir, "attestation.json"), []byte("{not json"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			s, err := status.Read(dir)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.LastStatus != "passed" || s.LastBundleID != tt.evidenceBundleID {
				t.Fatalf("expected evidence-sourced status/bundle, got %q / %q", s.LastStatus, s.LastBundleID)
			}
			if tt.wantTimestamp && !s.LastVerifiedAt.Equal(verifiedAt) {
				t.Fatalf("expected LastVerifiedAt %v, got %v", verifiedAt, s.LastVerifiedAt)
			}
			if !tt.wantTimestamp && !s.LastVerifiedAt.IsZero() {
				t.Fatalf("expected zero LastVerifiedAt, got %v", s.LastVerifiedAt)
			}
		})
	}
}

func writeVerificationFiles(t *testing.T, apDir, evidenceBundleID, attestationBundleID string, verifiedAt time.Time) {
	t.Helper()
	evidenceBytes, err := json.Marshal(evidence.Run{Status: "passed", BundleID: evidenceBundleID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "evidence.json"), evidenceBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	attestationBytes, err := json.Marshal(evidence.Attestation{BundleID: attestationBundleID, CreatedAt: verifiedAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "attestation.json"), attestationBytes, 0o600); err != nil {
		t.Fatal(err)
	}
}

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
