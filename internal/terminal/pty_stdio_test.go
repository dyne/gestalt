package terminal

import (
	"bytes"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestStdioPtyRoundTrip(t *testing.T) {
	path, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not available")
	}
	pty, cmd, err := StdioPtyFactory().Start(path)
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	payload := []byte("hello stdio pty\n")
	if _, err := pty.Write(payload); err != nil {
		_ = pty.Close()
		t.Fatalf("write: %v", err)
	}

	read := make([]byte, 0, len(payload))
	buf := make([]byte, 32)
	deadline := time.Now().Add(2 * time.Second)
	for len(read) < len(payload) && time.Now().Before(deadline) {
		n, err := pty.Read(buf)
		if n > 0 {
			read = append(read, buf[:n]...)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			_ = pty.Close()
			t.Fatalf("read: %v", err)
		}
	}
	if !bytes.Equal(read[:len(payload)], payload) {
		_ = pty.Close()
		t.Fatalf("payload mismatch: %q", string(read))
	}

	_ = pty.Close()
	_ = cmd.Wait()
}
