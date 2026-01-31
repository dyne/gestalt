package terminal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gestalt/internal/agent"
)

func TestPromptInjectionMCPDoesNotWaitOnAir(testingContext *testing.T) {
	oldTimeout := onAirTimeout
	onAirTimeout = 200 * time.Millisecond
	defer func() {
		onAirTimeout = oldTimeout
	}()

	root := testingContext.TempDir()
	promptsDir := filepath.Join(root, "prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		testingContext.Fatalf("mkdir prompts: %v", err)
	}
	promptPath := filepath.Join(promptsDir, "greeting.txt")
	if err := os.WriteFile(promptPath, []byte("Hello"), 0644); err != nil {
		testingContext.Fatalf("write prompt: %v", err)
	}

	basePty, serverIn, serverOut := newPipePty()
	promptCh := make(chan string, 1)
	server := newFakeMCPServer(testingContext, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		if prompt, ok := args["prompt"].(string); ok {
			select {
			case promptCh <- prompt:
			default:
			}
		}
		server.send(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"threadId": "thread-1",
				"content":  "ok",
			},
		})
	}
	go server.run()

	mcp := newMCPPty(basePty, false)
	factory := &staticPtyFactory{pty: mcp}
	manager := NewManager(ManagerOptions{
		PtyFactory: factory,
		PromptDir:  promptsDir,
		Agents: map[string]agent.Agent{
			"codex": {
				Name:        "Codex",
				CLIType:     "codex",
				CodexMode:   agent.CodexModeMCPServer,
				Prompts:     agent.PromptList{"greeting"},
				OnAirString: "READY",
			},
		},
	})

	session, err := manager.Create("codex", "run", "prompt")
	if err != nil {
		testingContext.Fatalf("create session: %v", err)
	}
	defer func() {
		_ = manager.Delete(session.ID)
	}()

	select {
	case prompt := <-promptCh:
		if prompt != "Hello" {
			testingContext.Fatalf("expected prompt Hello, got %q", prompt)
		}
	case <-time.After(1 * time.Second):
		testingContext.Fatalf("timed out waiting for prompt injection")
	}
}
