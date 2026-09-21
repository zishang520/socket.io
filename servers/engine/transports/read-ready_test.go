package transports

import (
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Observe the reader's cleanup independently of Once removing its own listener.
// The emitter performs that removal directly, bypassing this override.
type readReadyTestTransport struct {
	Transport
	removed   chan struct{}
	readState func(string)
}

func (t *readReadyTestTransport) RemoveListener(event types.EventName, listener types.EventListener) bool {
	removed := t.Transport.RemoveListener(event, listener)
	select {
	case t.removed <- struct{}{}:
	default:
	}
	return removed
}

func (t *readReadyTestTransport) ReadyState() string {
	state := t.Transport.ReadyState()
	if t.readState != nil {
		t.readState(state)
	}
	return state
}

func newReadReadyTestTransport(t *testing.T) *readReadyTestTransport {
	t.Helper()
	transport := &readReadyTestTransport{
		Transport: MakeTransport(),
		removed:   make(chan struct{}, 1),
	}
	t.Cleanup(transport.OnClose)
	return transport
}

func awaitReadReadyEvent(t *testing.T, event <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func assertReadReadyNotStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
		t.Fatal("reader started after startup was canceled")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestStartReaderWithoutReadinessBarrier(t *testing.T) {
	transport := newReadReadyTestTransport(t)
	started := make(chan struct{}, 1)
	StartReader(transport, nil, func() { started <- struct{}{} })
	awaitReadReadyEvent(t, started, "reader startup")
	if got := transport.ListenerCount("close"); got != 0 {
		t.Fatalf("default startup registered %d close listeners", got)
	}
}

func TestStartReaderWaitsForReadiness(t *testing.T) {
	transport := newReadReadyTestTransport(t)
	ready := make(chan bool, 1)
	started := make(chan struct{}, 1)
	checking := make(chan struct{}, 1)
	transport.readState = func(string) {
		select {
		case checking <- struct{}{}:
		default:
		}
	}

	StartReader(transport, ready, func() { started <- struct{}{} })
	if got := transport.ListenerCount("close"); got != 1 {
		t.Fatalf("startup must register cancellation synchronously, got %d listeners", got)
	}
	awaitReadReadyEvent(t, checking, "reader readiness check")
	select {
	case <-started:
		t.Fatal("reader started before initialization completed")
	case <-time.After(25 * time.Millisecond):
	}

	ready <- true
	close(ready)
	awaitReadReadyEvent(t, started, "reader startup after initialization")
	awaitReadReadyEvent(t, transport.removed, "readiness listener cleanup")
	if got := transport.ListenerCount("close"); got != 0 {
		t.Fatalf("startup retained %d close listeners", got)
	}
}

func TestStartReaderSkipsAlreadyClosingTransport(t *testing.T) {
	for _, state := range []string{"closing", "closed"} {
		t.Run(state, func(t *testing.T) {
			transport := newReadReadyTestTransport(t)
			transport.SetReadyState(state)
			ready := make(chan bool, 1)
			started := make(chan struct{}, 1)
			StartReader(transport, ready, func() { started <- struct{}{} })

			awaitReadReadyEvent(t, transport.removed, "canceled reader cleanup")
			assertReadReadyNotStarted(t, started)
			if got := transport.ListenerCount("close"); got != 0 {
				t.Fatalf("canceled startup retained %d close listeners", got)
			}
		})
	}
}

func TestStartReaderCloseBeforeReadiness(t *testing.T) {
	transport := newReadReadyTestTransport(t)
	ready := make(chan bool, 1)
	started := make(chan struct{}, 1)
	StartReader(transport, ready, func() { started <- struct{}{} })

	transport.OnClose()
	awaitReadReadyEvent(t, transport.removed, "canceled reader cleanup")
	if got := transport.ListenerCount("close"); got != 0 {
		t.Fatalf("closed transport retained %d close listeners", got)
	}
	ready <- true
	close(ready)
	assertReadReadyNotStarted(t, started)
}

func TestStartReaderDeniedInitialization(t *testing.T) {
	for _, alreadyDenied := range []bool{false, true} {
		t.Run(map[bool]string{false: "waiting", true: "late_constructor"}[alreadyDenied], func(t *testing.T) {
			transport := newReadReadyTestTransport(t)
			permission := make(chan bool)
			started := make(chan struct{}, 1)
			if alreadyDenied {
				close(permission)
			}
			StartReader(transport, permission, func() { started <- struct{}{} })
			if alreadyDenied {
				if transport.ReadyState() != "closing" {
					t.Fatal("late constructor did not close its transport")
				}
			} else {
				// The timeout closes permission before executing connection close
				// callbacks, which may block. Reader cancellation must not wait.
				close(permission)
				awaitReadReadyEvent(t, transport.removed, "denied reader cleanup")
			}
			assertReadReadyNotStarted(t, started)
		})
	}
}

func TestStartReaderCancelsBeforeBlockingCloseListener(t *testing.T) {
	transport := newReadReadyTestTransport(t)
	ready := make(chan bool, 1)
	started := make(chan struct{}, 1)
	StartReader(transport, ready, func() { started <- struct{}{} })

	entered := make(chan struct{})
	blocked := make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	_ = transport.Once("close", func(...any) {
		close(entered)
		<-blocked
	})
	closed := make(chan struct{})
	go func() {
		transport.OnClose()
		close(closed)
	}()

	awaitReadReadyEvent(t, entered, "blocking close listener")
	awaitReadReadyEvent(t, transport.removed, "reader cancellation during close callback")
	assertReadReadyNotStarted(t, started)
	release()
	awaitReadReadyEvent(t, closed, "transport close completion")
}

func TestStartReaderDoesNotRestartWhenReadyAndClosed(t *testing.T) {
	transport := newReadReadyTestTransport(t)
	ready := make(chan bool, 1)
	started := make(chan struct{}, 1)
	checked := make(chan struct{})
	blocked := make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	checkOnce := sync.OnceFunc(func() {
		close(checked)
		<-blocked
	})
	transport.readState = func(string) { checkOnce() }

	returned := make(chan struct{})
	go func() {
		StartReader(transport, ready, func() { started <- struct{}{} })
		close(returned)
	}()
	awaitReadReadyEvent(t, checked, "initial open-state snapshot")
	// Let the initial state check return its already captured "open" value
	// after both signals become ready. The state must be checked again.
	ready <- true
	close(ready)
	transport.OnClose()
	transport.Emit("close")
	release()

	awaitReadReadyEvent(t, returned, "reader startup registration")
	awaitReadReadyEvent(t, transport.removed, "closed reader cleanup")
	assertReadReadyNotStarted(t, started)
}
