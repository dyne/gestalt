package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"

	"gestalt/internal/agent"
	"gestalt/internal/ports"
	"gestalt/internal/prompt"
)

const promptSeparator = "\n\n"

func renderDeveloperPrompt(agent agent.Agent, promptFS fs.FS, root string, resolver ports.PortResolver) (string, error) {
	if len(agent.Prompts) == 0 {
		return "", nil
	}
	promptDir := path.Join(root, "prompts")
	parser := prompt.NewParser(promptFS, promptDir, ".", resolver)
	var buffer bytes.Buffer
	for i, promptName := range agent.Prompts {
		result, err := parser.Render(promptName)
		if err != nil {
			return "", fmt.Errorf("render prompt %q from %s: %w", promptName, promptDir, err)
		}
		if i > 0 {
			buffer.WriteString(promptSeparator)
		}
		buffer.Write(result.Content)
	}
	return buffer.String(), nil
}
