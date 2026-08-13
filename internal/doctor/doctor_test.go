package doctor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ralabarta/agentproof/internal/doctor"
)

func TestDoctor_Uninitialized(t *testing.T) {
	dir := t.TempDir()
	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Name == "agentproof-init" && f.Severity == doctor.SeverityWarn {
			found = true
		}
	}
	if !found {
		t.Fatal("expected agentproof-init warn finding for uninitialized dir")
	}
	if !report.Healthy {
		t.Fatal("expected Healthy=true when only warn findings present")
	}
}

func TestDoctor_Initialized(t *testing.T) {
	dir := t.TempDir()
	apDir := filepath.Join(dir, ".agentproof")
	os.MkdirAll(apDir, 0o700)
	os.WriteFile(filepath.Join(apDir, "config.json"), []byte(`{}`), 0o600)

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range report.Findings {
		if f.Name == "agentproof-init" && f.Severity != doctor.SeverityOK {
			t.Fatalf("expected agentproof-init ok for initialized dir, got %s", f.Severity)
		}
	}
}

// A run whose state.json stays "recording" after its process is gone (a crash
// that bypassed signal handling, such as SIGKILL) must surface as a warning.
func TestDoctor_WarnsOnStuckRecordingRuns(t *testing.T) {
	dir := initialized(t)
	if err := os.MkdirAll(filepath.Join(dir, ".agentproof", "runs", "stuck"), 0o700); err != nil {
		t.Fatal(err)
	}
	stateData, _ := json.Marshal(map[string]string{"status": "recording"})
	if err := os.WriteFile(filepath.Join(dir, ".agentproof", "runs", "stuck", "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report, "stuck-recording-runs", doctor.SeverityWarn) {
		t.Fatal("expected a stuck-recording-runs warn finding")
	}
}

// A "recording" run that still owns a live lock is a record in progress, not
// a stuck run, and must not be reported.
func TestDoctor_SilentWhileRecordingRunIsLive(t *testing.T) {
	dir := initialized(t)
	if err := os.MkdirAll(filepath.Join(dir, ".agentproof", "runs", "live"), 0o700); err != nil {
		t.Fatal(err)
	}
	stateData, _ := json.Marshal(map[string]string{"status": "recording"})
	if err := os.WriteFile(filepath.Join(dir, ".agentproof", "runs", "live", "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	// The test process itself owns the lock, so the recording run is live.
	if err := os.WriteFile(filepath.Join(dir, ".agentproof", ".record.lock"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(report, "stuck-recording-runs", doctor.SeverityWarn) {
		t.Fatal("a live record must not be reported as stuck")
	}
}

func initialized(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agentproof"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentproof", "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func hasFinding(report doctor.Report, name string, severity doctor.Severity) bool {
	for _, f := range report.Findings {
		if f.Name == name && f.Severity == severity {
			return true
		}
	}
	return false
}
