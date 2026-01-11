package terminal

import (
	"bytes"
	"testing"
	"time"
)

func receiveChunk(t *testing.T, ch <-chan []byte, expected []byte) bool {
	t.Helper()
	select {
	case got := <-ch:
		return bytes.Equal(got, expected)
	case <-time.After(200 * time.Millisecond):
		return false
	}
}
