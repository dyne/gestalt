//go:build !windows

package process

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRegistryStopsProcess(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
	}()

	registry := NewRegistry()
	registry.RegisterWithWait(cmd.Process.Pid, GroupID(cmd.Process.Pid), "sleep", func(ctx context.Context) error {
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := registry.StopAll(ctx); err != nil {
		t.Fatalf("stop all: %v", err)
	}

	if err := syscall.Kill(cmd.Process.Pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
		t.Fatalf("expected process to exit")
	}
}

func TestRegistryIgnoresExitedProcess(t *testing.T) {
	cmd := exec.Command("sleep", "0.1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = cmd.Wait()

	registry := NewRegistry()
	registry.Register(cmd.Process.Pid, GroupID(cmd.Process.Pid), "sleep")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := registry.StopAll(ctx); err != nil {
		t.Fatalf("stop all: %v", err)
	}
}
