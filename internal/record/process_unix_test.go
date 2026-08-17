//go:build !windows

package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ralabarta/agentproof/internal/config"
)

const processSignalHelperEnv = "AP_PROCESS_SIGNAL_HELPER"

func TestTargetedSIGTERMReachesChildProcessGroup(t *testing.T) {
	testTargetedSignalReachesChildProcessGroup(t, syscall.SIGTERM)
}

func TestTargetedSIGINTReachesChildProcessGroup(t *testing.T) {
	testTargetedSignalReachesChildProcessGroup(t, syscall.SIGINT)
}

func TestSignalWritesAbandonedStateBeforeWrapperExit(t *testing.T) {
	harness := startProcessSignalHarness(t)
	harness.signalWrapper(t, syscall.SIGTERM)
	harness.waitForWrapperSignalExit(t, syscall.SIGTERM)

	state := waitForProcessSignalState(t, harness.root, "abandoned")
	if state.Signal != "SIGTERM" {
		t.Fatalf("abandoned state signal = %q, want SIGTERM", state.Signal)
	}
	for _, result := range []string{harness.childResult, harness.grandchildResult} {
		got := waitForProcessSignalFile(t, result)
		if got != "SIGTERM abandoned" {
			t.Fatalf("signal receipt %s = %q, want %q", filepath.Base(result), got, "SIGTERM abandoned")
		}
	}
}

func TestSignalReportsAbandonedStateWriteFailure(t *testing.T) {
	harness := startProcessSignalHarness(t)
	harness.replaceStateWithDirectory(t)
	harness.signalWrapper(t, syscall.SIGTERM)
	harness.waitForWrapperSignalExit(t, syscall.SIGTERM)

	for _, result := range []string{harness.childResult, harness.grandchildResult} {
		got := waitForProcessSignalFile(t, result)
		if !strings.HasPrefix(got, "SIGTERM ") {
			t.Fatalf("signal receipt %s = %q, want SIGTERM receipt", filepath.Base(result), got)
		}
	}
	if got := harness.stderr.String(); !strings.Contains(got, "agentproof: abandoned-state publication failed\n") {
		t.Fatalf("wrapper stderr = %q, want abandoned-state publication diagnostic", got)
	}
}

func TestWindowsProcessConfigurationDoesNotCreateNewProcessGroup(t *testing.T) {
	source, err := os.ReadFile("process_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"CREATE_NEW_PROCESS_GROUP", "createNewProcessGroup", "CreationFlags:"} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("process_windows.go still contains unsupported process isolation %q", forbidden)
		}
	}
}

func TestProcessSignalForwardingDiagnostic(t *testing.T) {
	tests := []struct {
		name   string
		result processSignalForwarding
		want   string
	}{
		{name: "forwarded", result: processSignalForwarded},
		{name: "not running", result: processSignalNotRunning, want: "agentproof: signal forwarding failed: child process is not running"},
		{name: "unsupported", result: processSignalUnsupported, want: "agentproof: signal forwarding failed: unsupported signal or platform"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processSignalForwardingDiagnostic(tt.result); got != tt.want {
				t.Fatalf("processSignalForwardingDiagnostic(%d) = %q, want %q", tt.result, got, tt.want)
			}
		})
	}
}

func testTargetedSignalReachesChildProcessGroup(t *testing.T, received syscall.Signal) {
	t.Helper()
	harness := startProcessSignalHarness(t)
	harness.signalWrapper(t, received)
	harness.waitForWrapperSignalExit(t, received)

	want := signalName(received) + " abandoned"
	for _, result := range []string{harness.childResult, harness.grandchildResult} {
		if got := waitForProcessSignalFile(t, result); got != want {
			t.Fatalf("signal receipt %s = %q, want %q", filepath.Base(result), got, want)
		}
	}
}

type processSignalHarness struct {
	root             string
	cmd              *exec.Cmd
	done             chan error
	waited           bool
	stderr           bytes.Buffer
	childReady       string
	grandchildReady  string
	childResult      string
	grandchildResult string
}

func startProcessSignalHarness(t *testing.T) *processSignalHarness {
	t.Helper()
	dir := t.TempDir()
	harness := &processSignalHarness{
		root:             newRepo(t),
		done:             make(chan error, 1),
		childReady:       filepath.Join(dir, "child.ready"),
		grandchildReady:  filepath.Join(dir, "grandchild.ready"),
		childResult:      filepath.Join(dir, "child.result"),
		grandchildResult: filepath.Join(dir, "grandchild.result"),
	}
	harness.cmd = exec.Command(os.Args[0], "-test.run=^TestProcessSignalHelper$")
	harness.cmd.Env = append(os.Environ(),
		processSignalHelperEnv+"=wrapper",
		"AP_PROCESS_SIGNAL_ROOT="+harness.root,
		"AP_PROCESS_SIGNAL_CHILD_READY="+harness.childReady,
		"AP_PROCESS_SIGNAL_GRANDCHILD_READY="+harness.grandchildReady,
		"AP_PROCESS_SIGNAL_CHILD_RESULT="+harness.childResult,
		"AP_PROCESS_SIGNAL_GRANDCHILD_RESULT="+harness.grandchildResult,
	)
	harness.cmd.Stdout = os.Stdout
	harness.cmd.Stderr = &harness.stderr
	if err := harness.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { harness.done <- harness.cmd.Wait() }()
	t.Cleanup(func() { harness.cleanup(t) })
	waitForProcessSignalFile(t, harness.childReady)
	waitForProcessSignalFile(t, harness.grandchildReady)
	waitForProcessSignalState(t, harness.root, "recording")
	return harness
}

func (h *processSignalHarness) replaceStateWithDirectory(t *testing.T) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(h.root, config.DirName, "runs", "*", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("state paths = %v, want exactly one", matches)
	}
	if err := os.Remove(matches[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(matches[0], 0o700); err != nil {
		t.Fatal(err)
	}
}

func (h *processSignalHarness) signalWrapper(t *testing.T, received syscall.Signal) {
	t.Helper()
	if err := h.cmd.Process.Signal(received); err != nil {
		t.Fatalf("signal wrapper: %v", err)
	}
}

func (h *processSignalHarness) waitForWrapperSignalExit(t *testing.T, want syscall.Signal) {
	t.Helper()
	select {
	case err := <-h.done:
		h.waited = true
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("wrapper exit = %v, want signal %s", err, want)
		}
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != want {
			t.Fatalf("wrapper status = %v, want signal %s", status, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for wrapper signal exit")
	}
}

func (h *processSignalHarness) cleanup(t *testing.T) {
	t.Helper()
	killProcessSignalPID(t, h.childReady)
	killProcessSignalPID(t, h.grandchildReady)
	if !h.waited {
		if err := h.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill wrapper: %v", err)
		}
		select {
		case <-h.done:
		case <-time.After(2 * time.Second):
			t.Error("timed out reaping wrapper after kill")
		}
	}
}

func killProcessSignalPID(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Errorf("parse helper pid from %s: %v", path, err)
		return
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Errorf("kill helper pid %d: %v", pid, err)
	}
}

func waitForProcessSignalFile(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data))
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s (last error: %v)", path, lastErr)
	return ""
}

func waitForProcessSignalState(t *testing.T, root, want string) runState {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last runState
	var lastErr error
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(root, config.DirName, "runs", "*", "state.json"))
		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				lastErr = err
				continue
			}
			if err := json.Unmarshal(data, &last); err != nil {
				lastErr = err
				continue
			}
			if last.Status == want {
				return last
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %q (last state: %+v; last error: %v)", want, last, lastErr)
	return runState{}
}

func TestProcessSignalHelper(t *testing.T) {
	mode := os.Getenv(processSignalHelperEnv)
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "child" {
		mode = "child"
	}
	switch mode {
	case "":
		return
	case "wrapper":
		_, err := Run(os.Getenv("AP_PROCESS_SIGNAL_ROOT"), Options{
			Objective: "process signal forwarding",
			Command:   []string{os.Args[0], "-test.run=^TestProcessSignalHelper$", "--", "child"},
		})
		fmt.Fprintln(os.Stderr, err)
		os.Exit(41)
	case "child":
		runProcessSignalChild(t)
	case "grandchild":
		runProcessSignalLeaf(t, "AP_PROCESS_SIGNAL_GRANDCHILD_READY", "AP_PROCESS_SIGNAL_GRANDCHILD_RESULT")
	default:
		t.Fatalf("unknown process signal helper mode %q", os.Getenv(processSignalHelperEnv))
	}
}

func runProcessSignalChild(t *testing.T) {
	t.Helper()
	grandchild := exec.Command(os.Args[0], "-test.run=^TestProcessSignalHelper$")
	grandchild.Env = replaceProcessSignalHelperMode(os.Environ(), "grandchild")
	grandchild.Stdout = os.Stdout
	grandchild.Stderr = os.Stderr
	if err := grandchild.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("AP_PROCESS_SIGNAL_CHILD_READY"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForSignalAndPublishState(t, os.Getenv("AP_PROCESS_SIGNAL_CHILD_RESULT"))
}

func runProcessSignalLeaf(t *testing.T, readyEnv, resultEnv string) {
	t.Helper()
	if err := os.WriteFile(os.Getenv(readyEnv), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForSignalAndPublishState(t, os.Getenv(resultEnv))
}

func waitForSignalAndPublishState(t *testing.T, result string) {
	t.Helper()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	received := <-signals
	signal.Stop(signals)
	state := currentProcessSignalState(os.Getenv("AP_PROCESS_SIGNAL_ROOT"))
	if err := os.WriteFile(result, []byte(signalName(received)+" "+state), 0o600); err != nil {
		t.Fatal(err)
	}
}

func currentProcessSignalState(root string) string {
	matches, _ := filepath.Glob(filepath.Join(root, config.DirName, "runs", "*", "state.json"))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var state runState
		if json.Unmarshal(data, &state) == nil {
			return state.Status
		}
	}
	return "missing"
}

func replaceProcessSignalHelperMode(env []string, mode string) []string {
	prefix := processSignalHelperEnv + "="
	for i := range env {
		if strings.HasPrefix(env[i], prefix) {
			env[i] = prefix + mode
			return env
		}
	}
	return append(env, prefix+mode)
}
