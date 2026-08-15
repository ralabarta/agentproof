package record

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	signalStatePublicationTimeout = 250 * time.Millisecond
	signalDiagnosticTimeout       = 100 * time.Millisecond
)

var errLifecycleAbandoned = errors.New("record lifecycle is abandoning")

type lifecyclePhase uint8

const (
	lifecyclePreparing lifecyclePhase = iota
	lifecycleRunning
	lifecycleAbandoning
	lifecycleReaped
	lifecycleClosed
)

type lifecycleDependencies struct {
	writeState          func(string, runState) error
	forwardSignal       func(*os.Process, os.Signal) processSignalForwarding
	writeDiagnostic     func(string)
	raiseSignal         func(os.Signal) bool
	exitProcess         func(int)
	handlerExit         func()
	statePublishTimeout time.Duration
	diagnosticTimeout   time.Duration
}

type runLifecycle struct {
	mu      sync.Mutex
	stateMu sync.Mutex

	phase         lifecyclePhase
	child         *os.Process
	handlerActive bool
	handlerDone   chan struct{}
	stopHandlerCh chan struct{}

	statePath string
	started   time.Time
	deps      lifecycleDependencies
}

func newRunLifecycle(statePath string, started time.Time, deps lifecycleDependencies) *runLifecycle {
	if deps.writeState == nil {
		deps.writeState = writeRunState
	}
	if deps.forwardSignal == nil {
		deps.forwardSignal = forwardProcessSignal
	}
	if deps.writeDiagnostic == nil {
		deps.writeDiagnostic = func(message string) { fmt.Fprintln(os.Stderr, message) }
	}
	if deps.raiseSignal == nil {
		deps.raiseSignal = raiseSignal
	}
	if deps.exitProcess == nil {
		deps.exitProcess = os.Exit
	}
	if deps.handlerExit == nil {
		deps.handlerExit = func() {}
	}
	if deps.statePublishTimeout <= 0 {
		deps.statePublishTimeout = signalStatePublicationTimeout
	}
	if deps.diagnosticTimeout <= 0 {
		deps.diagnosticTimeout = signalDiagnosticTimeout
	}
	return &runLifecycle{
		phase:         lifecyclePreparing,
		handlerActive: true,
		handlerDone:   make(chan struct{}),
		stopHandlerCh: make(chan struct{}),
		statePath:     statePath,
		started:       started,
		deps:          deps,
	}
}

func (l *runLifecycle) startHandler(signals <-chan os.Signal) {
	go func() {
		defer close(l.handlerDone)
		defer l.deps.handlerExit()
		select {
		case received := <-signals:
			l.handleSignal(received)
		case <-l.stopHandlerCh:
		}
	}()
}

func (l *runLifecycle) publishRecording() error {
	return l.publishNormal(runState{Status: "recording", StartedAt: l.started}, lifecyclePreparing)
}

func (l *runLifecycle) publishComplete(state runState) error {
	return l.publishNormal(state, lifecycleClosed)
}

func (l *runLifecycle) publishNormal(state runState, required lifecyclePhase) error {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	l.mu.Lock()
	if l.phase == lifecycleAbandoning {
		l.mu.Unlock()
		return errLifecycleAbandoned
	}
	if l.phase != required {
		phase := l.phase
		l.mu.Unlock()
		return fmt.Errorf("invalid lifecycle phase %d for normal state publication", phase)
	}
	l.mu.Unlock()
	return l.deps.writeState(l.statePath, state)
}

func (l *runLifecycle) startCommand(start func() error, child func() *os.Process) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase == lifecycleAbandoning || !l.handlerActive {
		return errLifecycleAbandoned
	}
	if l.phase != lifecyclePreparing {
		return fmt.Errorf("invalid lifecycle phase %d for command start", l.phase)
	}
	if err := start(); err != nil {
		return err
	}
	l.child = child()
	l.phase = lifecycleRunning
	return nil
}

// waitAndReap is the sole child-reaping path. It clears child ownership under
// the forwarding lock immediately after Wait returns, before any result state
// can be published.
func (l *runLifecycle) waitAndReap(wait func() error) error {
	err := wait()
	l.mu.Lock()
	l.child = nil
	if l.phase == lifecycleRunning {
		l.phase = lifecycleReaped
	}
	l.mu.Unlock()
	return err
}

func (l *runLifecycle) stopHandler(stopNotify func()) {
	stopNotify()
	l.mu.Lock()
	if l.handlerActive {
		l.handlerActive = false
		close(l.stopHandlerCh)
	}
	done := l.handlerDone
	l.mu.Unlock()
	<-done

	l.mu.Lock()
	if l.phase != lifecycleAbandoning {
		l.phase = lifecycleClosed
	}
	l.mu.Unlock()
}

func (l *runLifecycle) handleSignal(received os.Signal) {
	l.mu.Lock()
	if !l.handlerActive || (l.phase != lifecyclePreparing && l.phase != lifecycleRunning) {
		l.mu.Unlock()
		return
	}
	l.phase = lifecycleAbandoning
	l.mu.Unlock()

	publication := make(chan error, 1)
	go func() {
		l.stateMu.Lock()
		defer l.stateMu.Unlock()
		publication <- l.deps.writeState(l.statePath, runState{
			Status:    "abandoned",
			StartedAt: l.started,
			Signal:    signalName(received),
		})
	}()

	publicationFailed := false
	select {
	case err := <-publication:
		publicationFailed = err != nil
	case <-time.After(l.deps.statePublishTimeout):
		publicationFailed = true
	}

	// Serialize child ownership checks with forwarding and clear ownership as soon
	// as Wait returns. A same-UID actor forcing PID/PGID reuse inside that narrow
	// post-reap window remains outside the portable stdlib threat boundary.
	l.mu.Lock()
	forwarding := l.deps.forwardSignal(l.child, received)
	l.mu.Unlock()

	var diagnostics []string
	if publicationFailed {
		diagnostics = append(diagnostics, abandonedStatePublicationDiagnostic)
	}
	if diagnostic := processSignalForwardingDiagnostic(forwarding); diagnostic != "" {
		diagnostics = append(diagnostics, diagnostic)
	}
	if len(diagnostics) > 0 {
		done := make(chan struct{})
		go func() {
			defer close(done)
			for _, diagnostic := range diagnostics {
				l.deps.writeDiagnostic(diagnostic)
			}
		}()
		select {
		case <-done:
		case <-time.After(l.deps.diagnosticTimeout):
		}
	}

	if l.deps.raiseSignal(received) {
		return
	}
	l.deps.exitProcess(1)
}
