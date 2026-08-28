package doctor_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestDoctorRecommendsValidRunPurgeCommand(t *testing.T) {
	dir := initialized(t)
	runDir := filepath.Join(dir, ".agentproof", "runs", "abandoned")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stateData, _ := json.Marshal(map[string]string{"status": "abandoned"})
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	finding, ok := findFinding(report, "abandoned-runs", doctor.SeverityWarn)
	if !ok {
		t.Fatal("expected an abandoned-runs warn finding")
	}
	want := "1 abandoned run(s) — consider running `agentproof purge --runs --older-than 0`"
	if finding.Detail != want {
		t.Fatalf("abandoned run guidance = %q, want %q", finding.Detail, want)
	}
	if strings.Contains(finding.Detail, "--confirm") {
		t.Fatal("first abandoned run recommendation must be a preview without --confirm")
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
	finding, ok := findFinding(report, "stuck-recording-runs", doctor.SeverityWarn)
	if !ok {
		t.Fatal("expected a stuck-recording-runs warn finding")
	}
	want := "1 run(s) stuck in the recording state — the record process died without completing; consider running `agentproof purge --runs --older-than 0`"
	if finding.Detail != want {
		t.Fatalf("stuck recording guidance = %q, want %q", finding.Detail, want)
	}
}

func TestDoctorIgnoresSymlinkedRecordingState(t *testing.T) {
	dir := initialized(t)
	runDir := filepath.Join(dir, ".agentproof", "runs", "active-recording")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "attacker-state.json")
	stateData, _ := json.Marshal(map[string]string{"status": "recording"})
	if err := os.WriteFile(target, stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(runDir, "state.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	startRecordLockHelper(t, dir, "active-recording")

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if hasFinding(report, "stuck-recording-runs", doctor.SeverityWarn) {
		t.Fatal("symlinked state must not be classified as a stuck recording run")
	}
}

func TestDoctorCountsOnlyMatchingLiveRecordingRun(t *testing.T) {
	dir := initialized(t)
	oldRun := filepath.Join(dir, ".agentproof", "runs", "old-recording")
	if err := os.MkdirAll(oldRun, 0o700); err != nil {
		t.Fatal(err)
	}
	stateData, _ := json.Marshal(map[string]string{"status": "recording"})
	if err := os.WriteFile(filepath.Join(oldRun, "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	startRecordLockHelper(t, dir, "active-recording")

	report, err := doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	finding, ok := findFinding(report, "stuck-recording-runs", doctor.SeverityWarn)
	if !ok {
		t.Fatal("a recording run with a different live lock owner must remain a warning")
	}
	if !strings.Contains(finding.Detail, "1 run(s)") {
		t.Fatalf("stuck recording detail = %q, want one run", finding.Detail)
	}

	activeRun := filepath.Join(dir, ".agentproof", "runs", "active-recording")
	if err := os.MkdirAll(activeRun, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeRun, "state.json"), stateData, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	finding, ok = findFinding(report, "stuck-recording-runs", doctor.SeverityWarn)
	if !ok || !strings.Contains(finding.Detail, "1 run(s)") {
		t.Fatalf("only the old run should remain stuck when the active run matches: %#v", report.Findings)
	}

	if err := os.WriteFile(filepath.Join(dir, ".agentproof", ".record.lock"), []byte("malformed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = doctor.Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	finding, ok = findFinding(report, "stuck-recording-runs", doctor.SeverityWarn)
	if !ok || !strings.Contains(finding.Detail, "2 run(s)") {
		t.Fatalf("malformed metadata must leave both recording runs visible: %#v", report.Findings)
	}
}

func startRecordLockHelper(t *testing.T, root, runID string) {
	t.Helper()
	control := t.TempDir()
	result := filepath.Join(control, "result")
	release := filepath.Join(control, "release")
	cmd := exec.Command("go", "test", "./internal/record", "-run=^TestRecordLockProcessHelper$", "-count=1")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(),
		"AP_RECORD_LOCK_HELPER=1",
		"AP_RECORD_LOCK_ROOT="+root,
		"AP_RECORD_LOCK_RUN_ID="+runID,
		"AP_RECORD_LOCK_MODE=hold",
		"AP_RECORD_LOCK_RESULT="+result,
		"AP_RECORD_LOCK_RELEASE="+release,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	helper := monitorRecordLockHelper(cmd)
	t.Cleanup(func() {
		cleanupRecordLockHelper(t, helper, release)
	})
	if got := waitForRecordLockHelper(t, helper, result); got != "acquired" {
		t.Fatalf("record lock helper result = %q, want acquired", got)
	}
}

type recordLockHelperProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func monitorRecordLockHelper(cmd *exec.Cmd) *recordLockHelperProcess {
	helper := &recordLockHelperProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		helper.err = cmd.Wait()
		close(helper.done)
	}()
	return helper
}

func cleanupRecordLockHelper(t *testing.T, helper *recordLockHelperProcess, release string) {
	t.Helper()
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		killAndReapRecordLockHelper(t, helper)
		t.Errorf("signal record lock helper release: %v", err)
		return
	}
	select {
	case <-helper.done:
		if helper.err != nil {
			t.Errorf("record lock helper failed: %v", helper.err)
		}
	case <-time.After(2 * time.Second):
		killAndReapRecordLockHelper(t, helper)
		t.Error("timed out waiting for record lock helper; killed and reaped process")
	}
}

func killAndReapRecordLockHelper(t *testing.T, helper *recordLockHelperProcess) {
	t.Helper()
	if err := helper.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("kill record lock helper: %v", err)
	}
	select {
	case <-helper.done:
	case <-time.After(2 * time.Second):
		t.Error("timed out reaping record lock helper after kill")
	}
}

func waitForRecordLockHelper(t *testing.T, helper *recordLockHelperProcess, path string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			last = strings.TrimSpace(string(data))
			if last == "acquired" {
				return last
			}
		} else {
			lastErr = err
		}
		select {
		case <-helper.done:
			t.Fatalf("record lock helper exited before publishing acquired: %v (last result %q, read error %v)", helper.err, last, lastErr)
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for record lock helper to publish acquired at %s (last result %q, read error %v)", path, last, lastErr)
	return ""
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
	_, ok := findFinding(report, name, severity)
	return ok
}

func findFinding(report doctor.Report, name string, severity doctor.Severity) (doctor.Finding, bool) {
	for _, f := range report.Findings {
		if f.Name == name && f.Severity == severity {
			return f, true
		}
	}
	return doctor.Finding{}, false
}
