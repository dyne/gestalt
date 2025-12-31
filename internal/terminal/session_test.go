package terminal

import (
	"errors"
	"testing"
	"time"
)

func TestSessionWriteAndOutput(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	out, cancel := session.Subscribe()
	defer cancel()

	pty.Emit("hello\n")
	if !receiveChunk(t, out, []byte("hello\n")) {
		t.Fatalf("expected to receive output chunk")
	}

	if err := session.Write([]byte("ls\n")); err != nil {
		t.Fatalf("write session: %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		pty.mu.Lock()
		writes := append([][]byte(nil), pty.writes...)
		pty.mu.Unlock()
		if len(writes) > 0 {
			if string(writes[0]) != "ls\n" {
				t.Fatalf("expected write ls\\n, got %q", string(writes[0]))
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for PTY write")
}

func TestSessionCloseTransitionsState(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, nil, nil)

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	if session.State() != sessionStateClosed {
		t.Fatalf("expected state closed, got %v", session.State())
	}
}

func TestSessionWriteAfterClose(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, nil, nil)

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected no panic, got %v", r)
		}
	}()

	if err := session.Write([]byte("ls\n")); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed, got %v", err)
	}
}

func TestSessionAutoRespondsToCursorPosition(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	pty.Emit("\x1b[6n")

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		pty.mu.Lock()
		writes := append([][]byte(nil), pty.writes...)
		pty.mu.Unlock()
		for _, write := range writes {
			if string(write) == "\x1b[1;1R" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected cursor position response")
}

func TestSessionRecordsInputHistory(t *testing.T) {
	pty := newScriptedPty()
	session := newSession("1", pty, nil, "title", "role", time.Now(), 10, nil, nil, nil)
	defer func() {
		_ = session.Close()
	}()

	session.RecordInput(" ls ")
	session.RecordInput("   ")

	entries := session.GetInputHistory()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", entries)
	}
	if entries[0].Command != "ls" {
		t.Fatalf("expected command ls, got %q", entries[0].Command)
	}
	if entries[0].Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
}
