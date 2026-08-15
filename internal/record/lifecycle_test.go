package record

import (
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestAbandonedCannotBeOverwrittenByComplete(t *testing.T) {
	var mu sync.Mutex
	var statuses []string
	lc := newRunLifecycle("state.json", time.Unix(0, 0).UTC(), lifecycleDependencies{
		writeState: func(_ string, state runState) error {
			mu.Lock()
			defer mu.Unlock()
			statuses = append(statuses, state.Status)
			return nil
		},
		forwardSignal:   func(*os.Process, os.Signal) processSignalForwarding { return processSignalNotRunning },
		writeDiagnostic: func(string) {},
		raiseSignal:     func(os.Signal) bool { return true },
	})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	lc.handleSignal(syscall.SIGTERM)

	finished := time.Unix(1, 0).UTC()
	if err := lc.publishComplete(runState{Status: "complete", CompletedAt: &finished}); !errors.Is(err, errLifecycleAbandoned) {
		t.Fatalf("publishComplete error = %v, want errLifecycleAbandoned", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(statuses) != 1 || statuses[0] != "abandoned" {
		t.Fatalf("published statuses = %v, want only abandoned", statuses)
	}
}

func TestHandlerJoinedBeforeRunReturns(t *testing.T) {
	exitStarted := make(chan struct{})
	releaseExit := make(chan struct{})
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{
		writeState:      func(string, runState) error { return nil },
		forwardSignal:   func(*os.Process, os.Signal) processSignalForwarding { return processSignalNotRunning },
		writeDiagnostic: func(string) {},
		raiseSignal:     func(os.Signal) bool { return true },
		handlerExit: func() {
			close(exitStarted)
			<-releaseExit
		},
	})
	signals := make(chan os.Signal, 1)
	lc.startHandler(signals)

	returned := make(chan struct{})
	go func() {
		lc.stopHandler(func() {})
		close(returned)
	}()
	<-exitStarted
	select {
	case <-returned:
		t.Fatal("stopHandler returned before the handler exited")
	default:
	}
	close(releaseExit)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("stopHandler did not return after the handler exited")
	}
}

func TestReapedChildIsNeverForwarded(t *testing.T) {
	forwarded := 0
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{
		writeState: func(string, runState) error { return nil },
		forwardSignal: func(*os.Process, os.Signal) processSignalForwarding {
			forwarded++
			return processSignalForwarded
		},
		writeDiagnostic: func(string) {},
		raiseSignal:     func(os.Signal) bool { return true },
	})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	if err := lc.waitAndReap(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	lc.handleSignal(syscall.SIGTERM)
	if forwarded != 0 {
		t.Fatalf("forward count = %d, want 0", forwarded)
	}
}

func TestBlockedStatePublicationStillForwardsAndTerminates(t *testing.T) {
	publicationStarted := make(chan struct{})
	releasePublication := make(chan struct{})
	forwarded := make(chan struct{})
	terminated := make(chan struct{})
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{
		writeState: func(string, runState) error {
			close(publicationStarted)
			<-releasePublication
			return nil
		},
		forwardSignal: func(*os.Process, os.Signal) processSignalForwarding {
			close(forwarded)
			return processSignalForwarded
		},
		writeDiagnostic:     func(string) {},
		raiseSignal:         func(os.Signal) bool { close(terminated); return true },
		statePublishTimeout: 20 * time.Millisecond,
		diagnosticTimeout:   20 * time.Millisecond,
	})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	go lc.handleSignal(syscall.SIGTERM)
	<-publicationStarted
	select {
	case <-forwarded:
	case <-time.After(time.Second):
		t.Fatal("blocked abandoned publication prevented signal forwarding")
	}
	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("blocked abandoned publication prevented exact termination")
	}
	close(releasePublication)
}

func TestBlockedDiagnosticDoesNotDelayTermination(t *testing.T) {
	diagnosticStarted := make(chan struct{})
	releaseDiagnostic := make(chan struct{})
	terminated := make(chan struct{})
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{
		writeState: func(string, runState) error { return errors.New("write failed") },
		forwardSignal: func(*os.Process, os.Signal) processSignalForwarding {
			return processSignalForwarded
		},
		writeDiagnostic: func(string) {
			close(diagnosticStarted)
			<-releaseDiagnostic
		},
		raiseSignal:       func(os.Signal) bool { close(terminated); return true },
		diagnosticTimeout: 20 * time.Millisecond,
	})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	go lc.handleSignal(syscall.SIGTERM)
	<-diagnosticStarted
	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("blocked diagnostic delayed exact termination")
	}
	close(releaseDiagnostic)
}

func TestAbandonedPublicationPrecedesForwarding(t *testing.T) {
	var mu sync.Mutex
	var events []string
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{
		writeState: func(_ string, state runState) error {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, state.Status)
			return nil
		},
		forwardSignal: func(*os.Process, os.Signal) processSignalForwarding {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, "forwarded")
			return processSignalForwarded
		},
		writeDiagnostic: func(string) {},
		raiseSignal:     func(os.Signal) bool { return true },
	})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	lc.handleSignal(syscall.SIGTERM)
	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0] != "abandoned" || events[1] != "forwarded" {
		t.Fatalf("events = %v, want [abandoned forwarded]", events)
	}
}

func startLifecycle(t *testing.T, lc *runLifecycle, process *os.Process) {
	t.Helper()
	if err := lc.startCommand(func() error { return nil }, func() *os.Process { return process }); err != nil {
		t.Fatal(err)
	}
}

func TestCommandWaitCalledExactlyOnce(t *testing.T) {
	waits := 0
	lc := newRunLifecycle("state.json", time.Time{}, lifecycleDependencies{})
	startLifecycle(t, lc, &os.Process{Pid: 123})
	want := errors.New("wait result")
	if got := lc.waitAndReap(func() error {
		waits++
		return want
	}); !errors.Is(got, want) {
		t.Fatalf("waitAndReap error = %v, want %v", got, want)
	}
	if waits != 1 {
		t.Fatalf("wait count = %d, want 1", waits)
	}
}
