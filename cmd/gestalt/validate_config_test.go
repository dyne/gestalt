package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gestalt/internal/logging"
	"gestalt/internal/prompt"
)

func TestValidatePromptFilesLogsBinary(t *testing.T) {
	root := t.TempDir()
	promptsDir := filepath.Join(root, "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	promptPath := filepath.Join(promptsDir, "binary.txt")
	if err := os.WriteFile(promptPath, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	buffer := logging.NewLogBuffer(10)
	logger := logging.NewLoggerWithOutput(buffer, logging.LevelInfo, io.Discard)
	prompt.ValidatePromptFiles(root, logger)

	if !hasLogMessage(buffer.List(), "prompt file is not valid text") {
		t.Fatalf("expected warning for invalid prompt text")
	}
}
