package doctor_test

import (
	"os"
	"path/filepath"
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
