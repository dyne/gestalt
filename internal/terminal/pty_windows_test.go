//go:build windows

package terminal

import (
	"strings"
	"testing"
)

func TestWrapPtyStartErrorAddsConPTYHint(t *testing.T) {
	wrapped := wrapPtyStartError(errConPTYUnavailable)
	if wrapped == nil {
		t.Fatalf("expected wrapped error")
	}
	if !strings.Contains(wrapped.Error(), conPTYUnavailableHint) {
		t.Fatalf("expected ConPTY hint in error, got %q", wrapped.Error())
	}
}
