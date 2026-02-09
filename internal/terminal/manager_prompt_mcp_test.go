package terminal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gestalt/internal/agent"
)

func TestMCPDeveloperInstructionsInjected(testingContext *testing.T) {
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
	callCh := make(chan callInfo, 1)
	server := newFakeMCPServer(testingContext, serverIn, serverOut, nil)
	server.onCall = func(id int64, name string, args map[string]interface{}) {
		callCh <- callInfo{name: name, args: args}
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
				Name:      "Codex",
				CLIType:   "codex",
				CodexMode: agent.CodexModeMCPServer,
				Prompts:   agent.PromptList{"greeting"},
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

	if err := session.Write([]byte("Ping\r")); err != nil {
		testingContext.Fatalf("write session: %v", err)
	}

	select {
	case call := <-callCh:
		if call.name != "codex" {
			testingContext.Fatalf("expected codex tool call, got %q", call.name)
		}
		value, ok := call.args["developer-instructions"].(string)
		if !ok || value == "" {
			testingContext.Fatalf("expected developer instructions, got %#v", call.args["developer-instructions"])
		}
		if value != "Hello" {
			testingContext.Fatalf("expected prompt in developer instructions, got %q", value)
		}
	case <-time.After(2 * time.Second):
		testingContext.Fatalf("timed out waiting for MCP call")
	}
}
